//! The Go core as a separate process. Its channel is created here with a random name and serves one client,
//! which must be the started process; both sides speak JSON-RPC 2.0, one message per line, and the core answers
//! nothing before a hello with the token it got in its arguments. The interface never touches the channel: it
//! calls `core_call`.
//!
//! TUN needs administrator rights, which only the core gets: `elevate` starts a core through UAC next to the
//! running one, which keeps working if the user says no. The new core waits for the old one to exit, so the data
//! folder, the port and the TUN are free when it takes them.
//!
//! A ready core is asked every few seconds whether it answers: one that stops answering is said to, and after a few
//! checks it is killed and started again, so that the interface never waits on a core that hangs.

use std::collections::{HashMap, VecDeque};
use std::path::{Path, PathBuf};
use std::sync::atomic::{AtomicU64, Ordering};
use std::sync::{Arc, Mutex};
use std::time::{Duration, Instant};

use serde::Serialize;
use serde_json::{Value, json};
use tokio::io::{AsyncBufReadExt, AsyncRead, AsyncWrite, AsyncWriteExt, BufReader};
use tokio::sync::{mpsc, oneshot, watch};

use crate::core_launch::{self, Arguments, ElevationError, Process};

const CALL_TIMEOUT: Duration = Duration::from_secs(30);
const CONNECT_TIMEOUT: Duration = Duration::from_secs(10);
/// A core that replaces another first waits up to 20 seconds for it to exit.
const HANDOVER_TIMEOUT: Duration = Duration::from_secs(45);
/// A core that ran this long before failing gets its restarts counted from zero again.
const STABLE_RUN: Duration = Duration::from_secs(30);
const MAX_RESTARTS: u32 = 5;
/// How long a core whose channel was closed gets to exit on its own: it closes its connection, TUN included, in
/// up to 15 seconds.
const EXIT_GRACE: Duration = Duration::from_secs(20);
/// The last lines the core wrote, to show why it stopped.
const TAIL_LINES: usize = 20;
const HEARTBEAT_EVERY: Duration = Duration::from_secs(5);
const HEARTBEAT_TIMEOUT: Duration = Duration::from_secs(5);
/// A core that misses this many checks in a row hangs: it is killed and started again.
const HEARTBEAT_MISSES: u32 = 3;
/// The exit code of a core that cannot read its data file (core/main.go): starting it again changes nothing.
const EXIT_DATA_DAMAGED: i64 = 4;
const DATA_FILE: &str = "poros.db";

/// Why the core is not working, beyond its state: its data file is damaged, or it stopped answering.
pub const REASON_DATA_DAMAGED: &str = "data_damaged";
pub const REASON_NOT_ANSWERING: &str = "not_answering";

#[derive(Serialize, Clone, Debug, PartialEq)]
#[serde(rename_all = "camelCase")]
pub enum CoreState {
    Starting,
    Ready,
    Down,
}

#[derive(Serialize, Clone, Debug)]
pub struct CoreStatus {
    pub state: CoreState,
    /// The core runs with administrator rights.
    pub elevated: bool,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub error: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub reason: Option<&'static str>,
}

/// The error object of a JSON-RPC response, or one made here when the core cannot answer.
#[derive(Serialize, Clone, Debug)]
pub struct CoreError {
    pub code: i64,
    pub message: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub data: Option<Value>,
}

/// Codes below the JSON-RPC range: the request never reached the core.
const CODE_NOT_RUNNING: i64 = -33000;
const CODE_TIMEOUT: i64 = -33001;
/// The user declined the UAC prompt; the running core stays.
pub const CODE_ELEVATION_CANCELLED: i64 = -33002;
/// The core could not be started, with or without rights.
pub const CODE_START_FAILED: i64 = -33003;
/// The updater could not be started; Poros runs on.
pub const CODE_UPDATE_FAILED: i64 = -33004;

impl CoreError {
    pub fn local(code: i64, message: impl Into<String>) -> Self {
        Self {
            code,
            message: message.into(),
            data: None,
        }
    }
}

/// Where the core's events and state changes go: the window in the app, a channel in tests.
pub trait Sink: Send + Sync + 'static {
    fn event(&self, method: &str, params: Value);
    fn status(&self, status: &CoreStatus);
}

pub struct CoreConfig {
    pub program: PathBuf,
    pub data_dir: PathBuf,
}

type Pending = Arc<Mutex<HashMap<u64, oneshot::Sender<Result<Value, CoreError>>>>>;

struct Link {
    tx: mpsc::UnboundedSender<String>,
    pending: Pending,
}

/// A request to replace the core: with rights for TUN, or without them.
struct Request {
    elevated: bool,
    window: Option<isize>,
    reply: oneshot::Sender<Result<(), CoreError>>,
}

struct Shared {
    link: Mutex<Option<Link>>,
    status: Mutex<CoreStatus>,
    next_id: AtomicU64,
    sink: Box<dyn Sink>,
    stop: watch::Sender<bool>,
    requests: mpsc::UnboundedSender<Request>,
    supervisor: Mutex<Option<tauri::async_runtime::JoinHandle<()>>>,
}

impl Shared {
    fn set_status(&self, state: CoreState, elevated: bool, error: Option<String>) {
        self.report(CoreStatus {
            state,
            elevated,
            error,
            reason: None,
        });
    }

    fn report(&self, status: CoreStatus) {
        {
            let mut current = self.status.lock().unwrap();
            if current.state != status.state
                || current.error != status.error
                || current.reason != status.reason
            {
                crate::applog::write(&describe(&status));
            }
            *current = status.clone();
        }
        self.sink.status(&status);
    }
}

/// A line of the log for a change of the core's state.
fn describe(status: &CoreStatus) -> String {
    let state = match status.state {
        CoreState::Starting => "starting",
        CoreState::Ready => "ready",
        CoreState::Down => "stopped",
    };
    let mut line = format!("shell: the core is {state}");
    if status.elevated {
        line.push_str(", with administrator rights");
    }
    if let Some(reason) = status.reason {
        line.push_str(&format!(" ({reason})"));
    }
    if let Some(error) = &status.error {
        line.push_str(&format!(": {error}"));
    }
    line
}

#[derive(Clone)]
pub struct Core {
    shared: Arc<Shared>,
    data_dir: PathBuf,
}

impl Core {
    /// Starts the core and keeps it running: a core that exits is started again, a few times in a row at most.
    pub fn start(config: CoreConfig, sink: impl Sink) -> Self {
        let (requests, received) = mpsc::unbounded_channel();
        let shared = Arc::new(Shared {
            link: Mutex::new(None),
            status: Mutex::new(CoreStatus {
                state: CoreState::Starting,
                elevated: false,
                error: None,
                reason: None,
            }),
            next_id: AtomicU64::new(1),
            sink: Box::new(sink),
            stop: watch::channel(false).0,
            requests,
            supervisor: Mutex::new(None),
        });
        let data_dir = config.data_dir.clone();
        let task = tauri::async_runtime::spawn(supervise(config, shared.clone(), received));
        *shared.supervisor.lock().unwrap() = Some(task);
        Self { shared, data_dir }
    }

    pub fn status(&self) -> CoreStatus {
        self.shared.status.lock().unwrap().clone()
    }

    /// Stops the core for good and waits until its process is gone.
    pub async fn shutdown(&self) {
        let _ = self.shared.stop.send(true);
        let task = self.shared.supervisor.lock().unwrap().take();
        if let Some(task) = task {
            let _ = task.await;
        }
    }

    /// Replaces the core with one that has administrator rights, after the UAC prompt over `window`. When the user
    /// declines, the error has `CODE_ELEVATION_CANCELLED` and the running core stays. The status tells when the
    /// new core is ready.
    pub async fn elevate(&self, window: Option<isize>) -> Result<(), CoreError> {
        self.replace(true, window).await
    }

    /// Replaces the core with one without rights; it also brings back a core that gave up after failing.
    pub async fn restart(&self) -> Result<(), CoreError> {
        self.replace(false, None).await
    }

    /// Puts a data file the core cannot read aside, as poros.db.damaged-<seconds>, and starts a core on clean data.
    /// The old file stays for the user; nothing is deleted.
    pub async fn reset_data(&self) -> Result<(), CoreError> {
        if self.status().reason != Some(REASON_DATA_DAMAGED) {
            return Err(CoreError::local(
                CODE_START_FAILED,
                "the data file is not damaged",
            ));
        }
        let file = self.data_dir.join(DATA_FILE);
        let stamp = std::time::SystemTime::now()
            .duration_since(std::time::UNIX_EPOCH)
            .map(|since| since.as_secs())
            .unwrap_or_default();
        let aside = self.data_dir.join(format!("{DATA_FILE}.damaged-{stamp}"));
        std::fs::rename(&file, &aside).map_err(|err| {
            CoreError::local(
                CODE_START_FAILED,
                format!("cannot move {}: {err}", file.display()),
            )
        })?;
        self.restart().await
    }

    async fn replace(&self, elevated: bool, window: Option<isize>) -> Result<(), CoreError> {
        let (reply, answer) = oneshot::channel();
        let request = Request {
            elevated,
            window,
            reply,
        };
        if self.shared.requests.send(request).is_err() {
            return Err(CoreError::local(CODE_NOT_RUNNING, "the core is stopped"));
        }
        answer
            .await
            .unwrap_or_else(|_| Err(CoreError::local(CODE_NOT_RUNNING, "the core is stopped")))
    }

    pub async fn call(&self, method: &str, params: Value) -> Result<Value, CoreError> {
        let (tx, pending) = {
            let link = self.shared.link.lock().unwrap();
            let link = link
                .as_ref()
                .ok_or_else(|| CoreError::local(CODE_NOT_RUNNING, "the core is not running"))?;
            (link.tx.clone(), link.pending.clone())
        };
        let id = self.shared.next_id.fetch_add(1, Ordering::Relaxed);
        let (reply_tx, reply_rx) = oneshot::channel();
        pending.lock().unwrap().insert(id, reply_tx);
        let message = json!({ "jsonrpc": "2.0", "id": id, "method": method, "params": params });
        if tx.send(message.to_string()).is_err() {
            pending.lock().unwrap().remove(&id);
            return Err(CoreError::local(
                CODE_NOT_RUNNING,
                "the core is not running",
            ));
        }
        match tokio::time::timeout(CALL_TIMEOUT, reply_rx).await {
            Ok(Ok(reply)) => reply,
            Ok(Err(_)) => Err(CoreError::local(
                CODE_NOT_RUNNING,
                "the core stopped before answering",
            )),
            Err(_) => {
                pending.lock().unwrap().remove(&id);
                Err(CoreError::local(
                    CODE_TIMEOUT,
                    format!("{method}: no answer in {CALL_TIMEOUT:?}"),
                ))
            }
        }
    }
}

type Tail = Arc<Mutex<VecDeque<String>>>;

/// A started core with its channel, before it connects.
struct Prepared {
    listener: channel::Listener,
    token: String,
    process: Process,
    stderr: Tail,
    /// It replaces a running core and connects only once that one has exited.
    handover: bool,
}

/// How a core's run ended: why, what kind of end it was, and the core that takes over, if one was asked for.
struct Outcome {
    reason: String,
    kind: Option<&'static str>,
    next: Option<Prepared>,
}

impl From<String> for Outcome {
    fn from(reason: String) -> Self {
        Self {
            reason,
            kind: None,
            next: None,
        }
    }
}

/// The end of a core whose process exited, with its last words; exit code 4 is a damaged data file.
fn exited(exit: core_launch::Exit, stderr: &Mutex<VecDeque<String>>, data_dir: &Path) -> Outcome {
    let kind = (exit.code == Some(EXIT_DATA_DAMAGED)).then_some(REASON_DATA_DAMAGED);
    Outcome {
        reason: with_tail(exit.text, stderr, data_dir),
        kind,
        next: None,
    }
}

async fn supervise(
    config: CoreConfig,
    shared: Arc<Shared>,
    mut requests: mpsc::UnboundedReceiver<Request>,
) {
    let mut stop = shared.stop.subscribe();
    let mut restarts = 0;
    let mut next: Option<Prepared> = None;
    loop {
        let prepared = match next.take() {
            Some(prepared) => prepared,
            None => match prepare(&config, false, None, None).await {
                Ok(prepared) => prepared,
                Err(err) => {
                    shared.set_status(CoreState::Down, false, Some(err.message));
                    match wait_for_request(&config, &shared, &mut stop, &mut requests).await {
                        Some(prepared) => prepared,
                        None => return,
                    }
                }
            },
        };
        let elevated = prepared.process.elevated();
        shared.set_status(CoreState::Starting, elevated, None);
        let started = Instant::now();
        let outcome = run_once(&config, &shared, &mut stop, &mut requests, prepared).await;
        if *stop.borrow() {
            shared.set_status(CoreState::Down, false, None);
            return;
        }
        if let Some(prepared) = outcome.next {
            next = Some(prepared);
            restarts = 0;
            continue;
        }
        // Another start would read the same file: the user chooses between clean data and leaving it.
        if outcome.kind == Some(REASON_DATA_DAMAGED) {
            shared.report(CoreStatus {
                state: CoreState::Down,
                elevated: false,
                error: Some(outcome.reason),
                reason: outcome.kind,
            });
            match wait_for_request(&config, &shared, &mut stop, &mut requests).await {
                Some(prepared) => {
                    restarts = 0;
                    next = Some(prepared);
                }
                None => return,
            }
            continue;
        }
        if started.elapsed() >= STABLE_RUN {
            restarts = 0;
        }
        restarts += 1;
        // An elevated core comes back without rights: asking for them again after every failure would put a UAC
        // prompt in front of the user each time. The interface offers to turn TUN back on.
        let reason = if elevated {
            format!(
                "the core with administrator rights stopped, so TUN is off: {}",
                outcome.reason
            )
        } else {
            outcome.reason
        };
        shared.report(CoreStatus {
            state: CoreState::Down,
            elevated: false,
            error: Some(reason),
            reason: outcome.kind,
        });
        if restarts > MAX_RESTARTS {
            match wait_for_request(&config, &shared, &mut stop, &mut requests).await {
                Some(prepared) => {
                    restarts = 0;
                    next = Some(prepared);
                }
                None => return,
            }
            continue;
        }
        tokio::select! {
            _ = tokio::time::sleep(Duration::from_secs(1 << restarts.min(4))) => {}
            _ = stopped(&mut stop) => return,
            Some(request) = requests.recv() => {
                next = answer(&config, request).await;
            }
        }
    }
}

/// Waits, with no core running, for a request to start one, or for the shutdown.
async fn wait_for_request(
    config: &CoreConfig,
    shared: &Shared,
    stop: &mut watch::Receiver<bool>,
    requests: &mut mpsc::UnboundedReceiver<Request>,
) -> Option<Prepared> {
    loop {
        tokio::select! {
            _ = stopped(stop) => {
                shared.set_status(CoreState::Down, false, None);
                return None;
            }
            request = requests.recv() => {
                let request = request?;
                if let Some(prepared) = answer(config, request).await {
                    return Some(prepared);
                }
            }
        }
    }
}

/// Starts the core a request asks for, with none running, and replies how it went.
async fn answer(config: &CoreConfig, request: Request) -> Option<Prepared> {
    match prepare(config, request.elevated, None, request.window).await {
        Ok(prepared) => {
            let _ = request.reply.send(Ok(()));
            Some(prepared)
        }
        Err(err) => {
            let _ = request.reply.send(Err(err));
            None
        }
    }
}

/// Waits for `shutdown`. The guard `wait_for` returns is not `Send`, so it must not live past this function.
async fn stopped(stop: &mut watch::Receiver<bool>) {
    let _ = stop.wait_for(|stopped| *stopped).await;
}

/// Creates a channel and starts a core for it; `replaces` is the core that must exit first.
async fn prepare(
    config: &CoreConfig,
    elevated: bool,
    replaces: Option<u32>,
    window: Option<isize>,
) -> Result<Prepared, CoreError> {
    let (name, token) = match (channel_name(), random_hex(16)) {
        (Ok(name), Ok(token)) => (name, token),
        (Err(err), _) | (_, Err(err)) => {
            return Err(CoreError::local(
                CODE_START_FAILED,
                format!("no random source: {err}"),
            ));
        }
    };
    let listener = channel::Listener::bind(&name).map_err(|err| {
        CoreError::local(
            CODE_START_FAILED,
            format!("cannot create the core channel: {err}"),
        )
    })?;
    let mut process = if elevated {
        let (program, data_dir, name, token) = (
            config.program.clone(),
            config.data_dir.clone(),
            name.clone(),
            token.clone(),
        );
        // The call waits for the user's answer in the UAC prompt.
        let started = tokio::task::spawn_blocking(move || {
            let args = Arguments {
                channel: &name,
                token: &token,
                data_dir: &data_dir,
                wait_pid: replaces,
            };
            core_launch::spawn_elevated(&program, &args, window)
        })
        .await
        .map_err(|err| CoreError::local(CODE_START_FAILED, err.to_string()))?;
        started.map_err(|err| match err {
            ElevationError::Cancelled => CoreError::local(
                CODE_ELEVATION_CANCELLED,
                "administrator rights were not given",
            ),
            ElevationError::Failed(message) => CoreError::local(CODE_START_FAILED, message),
        })?
    } else {
        let args = Arguments {
            channel: &name,
            token: &token,
            data_dir: &config.data_dir,
            wait_pid: replaces,
        };
        core_launch::spawn(&config.program, &args).map_err(|err| {
            CoreError::local(
                CODE_START_FAILED,
                format!("cannot start {}: {err}", config.program.display()),
            )
        })?
    };
    let stderr = collect_stderr(&mut process);
    Ok(Prepared {
        listener,
        token,
        process,
        stderr,
        handover: replaces.is_some(),
    })
}

/// Runs one core until it stops, `stop` comes, or another core is asked for, and says why it ended.
async fn run_once(
    config: &CoreConfig,
    shared: &Arc<Shared>,
    stop: &mut watch::Receiver<bool>,
    requests: &mut mpsc::UnboundedReceiver<Request>,
    prepared: Prepared,
) -> Outcome {
    let Prepared {
        listener,
        token,
        mut process,
        stderr,
        handover,
    } = prepared;
    let connect_timeout = if handover {
        HANDOVER_TIMEOUT
    } else {
        CONNECT_TIMEOUT
    };

    let stream = tokio::select! {
        accepted = tokio::time::timeout(connect_timeout, listener.accept()) => match accepted {
            Ok(Ok(stream)) => stream,
            Ok(Err(err)) => return format!("core channel: {err}").into(),
            Err(_) => {
                process.kill().await;
                return format!("the core did not connect in {connect_timeout:?}").into();
            }
        },
        exit = process.wait() => return exited(exit, &stderr, &config.data_dir),
        _ = stopped(stop) => {
            process.kill().await;
            return "stopped".to_string().into();
        }
    };
    #[cfg(windows)]
    if core_launch::client_pid(&stream) != process.pid() {
        process.kill().await;
        return "an unexpected process connected to the core channel"
            .to_string()
            .into();
    }

    let pending: Pending = Arc::default();
    let (tx, rx) = mpsc::unbounded_channel();
    let (reader, writer) = tokio::io::split(stream);
    let writer_task = tauri::async_runtime::spawn(write_lines(writer, rx));
    let mut reader_task =
        tauri::async_runtime::spawn(read_lines(reader, pending.clone(), shared.clone()));

    let hello_id = 0;
    let (hello_tx, hello_rx) = oneshot::channel();
    pending.lock().unwrap().insert(hello_id, hello_tx);
    let hello = json!({ "jsonrpc": "2.0", "id": hello_id, "method": "hello", "params": { "token": token } });
    let _ = tx.send(hello.to_string());

    // A core that cannot read its data exits before it answers the handshake.
    let handshake: Result<(), Outcome> = tokio::select! {
        reply = tokio::time::timeout(CALL_TIMEOUT, hello_rx) => match reply {
            Ok(Ok(Ok(_))) => Ok(()),
            Ok(Ok(Err(err))) => Err(format!("the core refused the handshake: {}", err.message).into()),
            _ => Err("the core did not answer the handshake".to_string().into()),
        },
        exit = process.wait() => Err(exited(exit, &stderr, &config.data_dir)),
        _ = stopped(stop) => Err("stopped".to_string().into()),
    };
    // Unanswered checks in a row; the core is killed at once on the way out when it does not answer.
    let mut misses = 0;
    let outcome = match handshake {
        Err(outcome) => outcome,
        Ok(()) => {
            let elevated = process.elevated();
            *shared.link.lock().unwrap() = Some(Link {
                tx: tx.clone(),
                pending: pending.clone(),
            });
            shared.set_status(CoreState::Ready, elevated, None);
            let mut heartbeat = tokio::time::interval(HEARTBEAT_EVERY);
            heartbeat.set_missed_tick_behavior(tokio::time::MissedTickBehavior::Delay);
            heartbeat.reset();
            loop {
                tokio::select! {
                    exit = process.wait() => break exited(exit, &stderr, &config.data_dir),
                    _ = &mut reader_task => break "the core closed its channel".to_string().into(),
                    _ = stopped(stop) => break "stopped".to_string().into(),
                    Some(request) = requests.recv() => {
                        match prepare(config, request.elevated, process.pid(), request.window).await {
                            Ok(next) => {
                                // Calls wait for the new core from here on; the status says so before the reply.
                                shared.link.lock().unwrap().take();
                                shared.set_status(CoreState::Starting, next.process.elevated(), None);
                                let _ = request.reply.send(Ok(()));
                                break Outcome { reason: "replaced".to_string(), kind: None, next: Some(next) };
                            }
                            Err(err) => {
                                let _ = request.reply.send(Err(err));
                            }
                        }
                    }
                    _ = heartbeat.tick() => {
                        let answered = tokio::select! {
                            answered = answers(&tx, &pending, shared) => answered,
                            _ = stopped(stop) => break "stopped".to_string().into(),
                        };
                        if answered {
                            if misses > 0 {
                                shared.set_status(CoreState::Ready, elevated, None);
                            }
                            misses = 0;
                            continue;
                        }
                        misses += 1;
                        if misses == 1 {
                            shared.report(CoreStatus {
                                state: CoreState::Ready,
                                elevated,
                                error: None,
                                reason: Some(REASON_NOT_ANSWERING),
                            });
                        }
                        if misses >= HEARTBEAT_MISSES {
                            break Outcome {
                                reason: format!("the core did not answer for {:?}", HEARTBEAT_EVERY * HEARTBEAT_MISSES),
                                kind: Some(REASON_NOT_ANSWERING),
                                next: None,
                            };
                        }
                    }
                }
            }
        }
    };

    shared.link.lock().unwrap().take();
    for (_, waiter) in pending.lock().unwrap().drain() {
        let _ = waiter.send(Err(CoreError::local(CODE_NOT_RUNNING, "the core stopped")));
    }
    // Closing both halves closes the channel, and a core that sees it closed stops its connection, saves its data
    // and exits. Killing it with a TUN up would leave the adapter and its routes behind for a while.
    writer_task.abort();
    reader_task.abort();
    // A core that does not answer would not close anything either: it goes at once.
    if misses > 0
        || tokio::time::timeout(EXIT_GRACE, process.wait())
            .await
            .is_err()
    {
        process.kill().await;
    }
    outcome
}

/// Asks the core for its version and tells whether the answer came in time.
async fn answers(tx: &mpsc::UnboundedSender<String>, pending: &Pending, shared: &Shared) -> bool {
    let id = shared.next_id.fetch_add(1, Ordering::Relaxed);
    let (reply_tx, reply_rx) = oneshot::channel();
    pending.lock().unwrap().insert(id, reply_tx);
    let message = json!({ "jsonrpc": "2.0", "id": id, "method": "app.info", "params": null });
    if tx.send(message.to_string()).is_err() {
        pending.lock().unwrap().remove(&id);
        return false;
    }
    let answered = matches!(
        tokio::time::timeout(HEARTBEAT_TIMEOUT, reply_rx).await,
        Ok(Ok(Ok(_)))
    );
    if !answered {
        pending.lock().unwrap().remove(&id);
    }
    answered
}

fn collect_stderr(process: &mut Process) -> Tail {
    let tail = Arc::new(Mutex::new(VecDeque::with_capacity(TAIL_LINES)));
    if let Process::Child(child) = process
        && let Some(stderr) = child.stderr.take()
    {
        let tail = tail.clone();
        tauri::async_runtime::spawn(async move {
            let mut lines = BufReader::new(stderr).lines();
            while let Ok(Some(line)) = lines.next_line().await {
                let mut tail = tail.lock().unwrap();
                if tail.len() == TAIL_LINES {
                    tail.pop_front();
                }
                tail.push_back(line);
            }
        });
    }
    tail
}

/// Adds the core's last words to why it stopped: its stderr, or else its log, which is all an elevated core has.
fn with_tail(reason: String, stderr: &Mutex<VecDeque<String>>, data_dir: &Path) -> String {
    let mut lines: Vec<String> = stderr.lock().unwrap().iter().cloned().collect();
    if lines.is_empty() {
        lines = log_tail(data_dir);
    }
    if lines.is_empty() {
        reason
    } else {
        format!("{reason}: {}", lines.join("\n"))
    }
}

fn log_tail(data_dir: &Path) -> Vec<String> {
    let Ok(text) = std::fs::read_to_string(data_dir.join("logs").join("core.log")) else {
        return Vec::new();
    };
    let lines: Vec<&str> = text.lines().collect();
    lines[lines.len().saturating_sub(TAIL_LINES)..]
        .iter()
        .map(|line| line.to_string())
        .collect()
}

async fn write_lines(mut writer: impl AsyncWrite + Unpin, mut rx: mpsc::UnboundedReceiver<String>) {
    while let Some(mut line) = rx.recv().await {
        line.push('\n');
        if writer.write_all(line.as_bytes()).await.is_err() {
            return;
        }
    }
}

async fn read_lines(reader: impl AsyncRead + Unpin, pending: Pending, shared: Arc<Shared>) {
    let mut lines = BufReader::new(reader).lines();
    while let Ok(Some(line)) = lines.next_line().await {
        let Ok(message) = serde_json::from_str::<Value>(&line) else {
            continue;
        };
        if let Some(id) = message.get("id").and_then(Value::as_u64) {
            let Some(waiter) = pending.lock().unwrap().remove(&id) else {
                continue;
            };
            let reply = match message.get("error") {
                Some(error) => Err(CoreError {
                    code: error.get("code").and_then(Value::as_i64).unwrap_or(0),
                    message: error
                        .get("message")
                        .and_then(Value::as_str)
                        .unwrap_or_default()
                        .to_string(),
                    data: error.get("data").cloned(),
                }),
                None => Ok(message.get("result").cloned().unwrap_or(Value::Null)),
            };
            let _ = waiter.send(reply);
        } else if let Some(method) = message.get("method").and_then(Value::as_str) {
            shared.sink.event(
                method,
                message.get("params").cloned().unwrap_or(Value::Null),
            );
        }
    }
}

fn random_hex(bytes: usize) -> Result<String, getrandom::Error> {
    let mut buf = vec![0u8; bytes];
    getrandom::fill(&mut buf)?;
    Ok(buf.iter().map(|b| format!("{b:02x}")).collect())
}

fn channel_name() -> Result<String, getrandom::Error> {
    let suffix = random_hex(8)?;
    #[cfg(windows)]
    return Ok(format!(r"\\.\pipe\poros-core-{suffix}"));
    #[cfg(unix)]
    return Ok(std::env::temp_dir()
        .join(format!("poros-core-{suffix}.sock"))
        .display()
        .to_string());
}

#[cfg(windows)]
mod channel {
    use tokio::net::windows::named_pipe::{NamedPipeServer, ServerOptions};

    use crate::core_launch::pipe_security;

    pub struct Listener(NamedPipeServer);

    impl Listener {
        /// A single instance that must be the first with this name, so nothing can sit in front of it, open to
        /// this account and administrators only.
        pub fn bind(name: &str) -> std::io::Result<Self> {
            let mut security = pipe_security::attributes()?;
            // SAFETY: the attributes and their descriptor live until create returns.
            unsafe {
                ServerOptions::new()
                    .first_pipe_instance(true)
                    .max_instances(1)
                    .reject_remote_clients(true)
                    .create_with_security_attributes_raw(
                        name,
                        (&raw mut security.attributes).cast(),
                    )
            }
            .map(Self)
        }

        pub async fn accept(self) -> std::io::Result<NamedPipeServer> {
            self.0.connect().await?;
            Ok(self.0)
        }
    }
}

#[cfg(unix)]
mod channel {
    use std::os::unix::fs::PermissionsExt;

    use tokio::net::{UnixListener, UnixStream};

    pub struct Listener(UnixListener, std::path::PathBuf);

    impl Listener {
        pub fn bind(name: &str) -> std::io::Result<Self> {
            let listener = UnixListener::bind(name)?;
            std::fs::set_permissions(name, std::fs::Permissions::from_mode(0o600))?;
            Ok(Self(listener, name.into()))
        }

        pub async fn accept(self) -> std::io::Result<UnixStream> {
            let (stream, _) = self.0.accept().await?;
            let _ = std::fs::remove_file(&self.1);
            Ok(stream)
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    struct Channel(mpsc::UnboundedSender<(String, Value)>);

    impl Sink for Channel {
        fn event(&self, method: &str, params: Value) {
            let _ = self.0.send((method.to_string(), params));
        }
        fn status(&self, _: &CoreStatus) {}
    }

    async fn wait_ready(core: &Core, elevated: bool, timeout: Duration) {
        let deadline = Instant::now() + timeout;
        loop {
            let status = core.status();
            if status.state == CoreState::Ready && status.elevated == elevated {
                return;
            }
            assert!(
                Instant::now() < deadline,
                "core did not become ready: {status:?}"
            );
            tokio::time::sleep(Duration::from_millis(50)).await;
        }
    }

    fn start_test_core(
        program: std::ffi::OsString,
    ) -> (Core, PathBuf, mpsc::UnboundedReceiver<(String, Value)>) {
        let data_dir =
            std::env::temp_dir().join(format!("poros-core-test-{}", random_hex(4).unwrap()));
        let core = start_test_core_with(program, data_dir.clone());
        (core.0, data_dir, core.1)
    }

    fn start_test_core_in(
        data_dir: PathBuf,
    ) -> (Core, PathBuf, mpsc::UnboundedReceiver<(String, Value)>) {
        let program = std::env::var_os("POROS_TEST_CORE").expect("POROS_TEST_CORE");
        let core = start_test_core_with(program, data_dir.clone());
        (core.0, data_dir, core.1)
    }

    fn start_test_core_with(
        program: std::ffi::OsString,
        data_dir: PathBuf,
    ) -> (Core, mpsc::UnboundedReceiver<(String, Value)>) {
        let (tx, events) = mpsc::unbounded_channel();
        let core = Core::start(
            CoreConfig {
                program: program.into(),
                data_dir,
            },
            Channel(tx),
        );
        (core, events)
    }

    /// Talks to a real core. Set POROS_TEST_CORE to the built PorosCore.exe to run it.
    #[tokio::test(flavor = "multi_thread")]
    async fn calls_the_core_and_receives_its_events() {
        let Some(program) = std::env::var_os("POROS_TEST_CORE") else {
            eprintln!("POROS_TEST_CORE is not set, skipping");
            return;
        };
        let (core, data_dir, mut events) = start_test_core(program);
        wait_ready(&core, false, CONNECT_TIMEOUT).await;

        let info = core.call("app.info", Value::Null).await.expect("app.info");
        assert!(
            info["singBox"].as_str().is_some_and(|v| !v.is_empty()),
            "{info}"
        );

        let error = core
            .call("no.such.method", Value::Null)
            .await
            .expect_err("unknown method");
        assert_eq!(error.code, -32601);

        let mut settings = core
            .call("settings.get", Value::Null)
            .await
            .expect("settings.get");
        settings["tun"] = Value::Bool(true);
        core.call("settings.set", settings)
            .await
            .expect("settings.set");
        let (method, params) = tokio::time::timeout(CALL_TIMEOUT, events.recv())
            .await
            .unwrap()
            .unwrap();
        assert_eq!(method, "settings.changed");
        assert_eq!(params["tun"], Value::Bool(true));

        // A new core takes over the data folder once the old one has let it go.
        core.restart().await.expect("restart");
        wait_ready(&core, false, HANDOVER_TIMEOUT).await;
        let settings = core
            .call("settings.get", Value::Null)
            .await
            .expect("settings.get after the restart");
        assert_eq!(settings["tun"], Value::Bool(true));
        assert!(
            data_dir.join("logs").join("core.log").exists(),
            "the core keeps its log in the data folder"
        );

        core.shutdown().await;
        assert_eq!(core.status().state, CoreState::Down);
        let error = core
            .call("app.info", Value::Null)
            .await
            .expect_err("a stopped core");
        assert_eq!(error.code, CODE_NOT_RUNNING);
        std::fs::remove_dir_all(&data_dir).expect("the stopped core must release its data folder");
    }

    /// Goes through UAC, so it runs only in the test machine with POROS_TEST_ELEVATE=1, where the prompt is
    /// turned off for the run.
    #[tokio::test(flavor = "multi_thread")]
    async fn elevates_the_core_and_comes_back_without_rights() {
        let (Some(program), Some(_)) = (
            std::env::var_os("POROS_TEST_CORE"),
            std::env::var_os("POROS_TEST_ELEVATE"),
        ) else {
            eprintln!("POROS_TEST_CORE and POROS_TEST_ELEVATE are not set, skipping");
            return;
        };
        let (core, data_dir, _events) = start_test_core(program);
        wait_ready(&core, false, CONNECT_TIMEOUT).await;
        let info = core.call("app.info", Value::Null).await.expect("app.info");
        assert_eq!(info["elevated"], Value::Bool(false));

        core.elevate(None).await.expect("elevate");
        wait_ready(&core, true, HANDOVER_TIMEOUT).await;
        let info = core.call("app.info", Value::Null).await.expect("app.info");
        assert_eq!(info["elevated"], Value::Bool(true), "{info}");

        core.restart().await.expect("restart");
        wait_ready(&core, false, HANDOVER_TIMEOUT).await;
        let info = core.call("app.info", Value::Null).await.expect("app.info");
        assert_eq!(info["elevated"], Value::Bool(false), "{info}");

        core.shutdown().await;
        std::fs::remove_dir_all(&data_dir).expect("the stopped core must release its data folder");
    }

    /// A data file the core cannot read stops it at once, without restarts, until the user asks for clean data;
    /// the old file stays beside. Set POROS_TEST_CORE to run it.
    #[tokio::test(flavor = "multi_thread")]
    async fn waits_for_the_user_on_damaged_data() {
        let Some(program) = std::env::var_os("POROS_TEST_CORE") else {
            eprintln!("POROS_TEST_CORE is not set, skipping");
            return;
        };
        let (core, data_dir, _events) = start_test_core(program);
        wait_ready(&core, false, CONNECT_TIMEOUT).await;
        core.shutdown().await;
        let file = data_dir.join(DATA_FILE);
        let mut data = std::fs::read(&file).expect("the data file");
        for (i, byte) in data.iter_mut().take(8192).enumerate() {
            *byte = (i * 7) as u8;
        }
        std::fs::write(&file, data).expect("damage the data file");

        let (core, _, _events) = start_test_core_in(data_dir.clone());
        let deadline = Instant::now() + CONNECT_TIMEOUT;
        while core.status().reason != Some(REASON_DATA_DAMAGED) {
            assert!(
                Instant::now() < deadline,
                "the damaged data was not told: {:?}",
                core.status()
            );
            tokio::time::sleep(Duration::from_millis(50)).await;
        }
        assert_eq!(core.status().state, CoreState::Down);
        // No restart comes on its own: the next would fail the same way.
        tokio::time::sleep(Duration::from_secs(3)).await;
        assert_eq!(core.status().reason, Some(REASON_DATA_DAMAGED));

        core.reset_data().await.expect("reset_data");
        wait_ready(&core, false, CONNECT_TIMEOUT).await;
        let aside = std::fs::read_dir(&data_dir)
            .unwrap()
            .filter_map(Result::ok)
            .filter(|entry| {
                entry
                    .file_name()
                    .to_string_lossy()
                    .starts_with("poros.db.damaged-")
            })
            .count();
        assert_eq!(aside, 1, "the damaged file stays beside the new one");
        assert!(
            core.reset_data().await.is_err(),
            "a core with good data keeps it"
        );
        core.shutdown().await;
        std::fs::remove_dir_all(&data_dir).expect("the stopped core must release its data folder");
    }
}
