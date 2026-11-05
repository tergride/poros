//! The Go core as a child process. Its channel is created here with a random name and serves one client;
//! both sides speak JSON-RPC 2.0, one message per line, and the core answers nothing before a hello with
//! the token it got in its environment. The interface never touches the channel: it calls `core_call`.

use std::collections::{HashMap, VecDeque};
use std::path::PathBuf;
use std::sync::atomic::{AtomicU64, Ordering};
use std::sync::{Arc, Mutex};
use std::time::{Duration, Instant};

use serde::Serialize;
use serde_json::{Value, json};
use tokio::io::{AsyncBufReadExt, AsyncRead, AsyncWrite, AsyncWriteExt, BufReader};
use tokio::process::{Child, Command};
use tokio::sync::{mpsc, oneshot, watch};

const CALL_TIMEOUT: Duration = Duration::from_secs(30);
const CONNECT_TIMEOUT: Duration = Duration::from_secs(10);
/// A core that ran this long before failing gets its restarts counted from zero again.
const STABLE_RUN: Duration = Duration::from_secs(30);
const MAX_RESTARTS: u32 = 5;
/// How long a core whose channel was closed gets to exit on its own.
const EXIT_GRACE: Duration = Duration::from_secs(3);
/// The last lines the core wrote to stderr, to show why it stopped.
const STDERR_TAIL: usize = 20;

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
    #[serde(skip_serializing_if = "Option::is_none")]
    pub error: Option<String>,
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

impl CoreError {
    fn local(code: i64, message: impl Into<String>) -> Self {
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

struct Shared {
    link: Mutex<Option<Link>>,
    status: Mutex<CoreStatus>,
    next_id: AtomicU64,
    sink: Box<dyn Sink>,
    stop: watch::Sender<bool>,
    supervisor: Mutex<Option<tauri::async_runtime::JoinHandle<()>>>,
}

impl Shared {
    fn set_status(&self, state: CoreState, error: Option<String>) {
        let status = CoreStatus { state, error };
        *self.status.lock().unwrap() = status.clone();
        self.sink.status(&status);
    }
}

#[derive(Clone)]
pub struct Core {
    shared: Arc<Shared>,
}

impl Core {
    /// Starts the core and keeps it running: a core that exits is started again, a few times in a row at most.
    pub fn start(config: CoreConfig, sink: impl Sink) -> Self {
        let shared = Arc::new(Shared {
            link: Mutex::new(None),
            status: Mutex::new(CoreStatus {
                state: CoreState::Starting,
                error: None,
            }),
            next_id: AtomicU64::new(1),
            sink: Box::new(sink),
            stop: watch::channel(false).0,
            supervisor: Mutex::new(None),
        });
        let task = tauri::async_runtime::spawn(supervise(config, shared.clone()));
        *shared.supervisor.lock().unwrap() = Some(task);
        Self { shared }
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

async fn supervise(config: CoreConfig, shared: Arc<Shared>) {
    let mut stop = shared.stop.subscribe();
    let mut restarts = 0;
    loop {
        shared.set_status(CoreState::Starting, None);
        let started = Instant::now();
        let reason = run_once(&config, &shared, &mut stop).await;
        if *stop.borrow() {
            shared.set_status(CoreState::Down, None);
            return;
        }
        if started.elapsed() >= STABLE_RUN {
            restarts = 0;
        }
        restarts += 1;
        shared.set_status(CoreState::Down, Some(reason));
        if restarts > MAX_RESTARTS {
            return;
        }
        tokio::select! {
            _ = tokio::time::sleep(Duration::from_secs(1 << restarts.min(4))) => {}
            _ = stopped(&mut stop) => {
                return;
            }
        }
    }
}

/// Waits for `shutdown`. The guard `wait_for` returns is not `Send`, so it must not live past this function.
async fn stopped(stop: &mut watch::Receiver<bool>) {
    let _ = stop.wait_for(|stopped| *stopped).await;
}

/// Runs one core until it stops, or until `stop`, and says why.
async fn run_once(
    config: &CoreConfig,
    shared: &Arc<Shared>,
    stop: &mut watch::Receiver<bool>,
) -> String {
    let (name, token) = match (channel_name(), random_hex(16)) {
        (Ok(name), Ok(token)) => (name, token),
        (Err(err), _) | (_, Err(err)) => return format!("no random source: {err}"),
    };
    let listener = match channel::Listener::bind(&name) {
        Ok(listener) => listener,
        Err(err) => return format!("cannot create the core channel: {err}"),
    };
    let mut child = match spawn(config, &name, &token) {
        Ok(child) => child,
        Err(err) => return format!("cannot start {}: {err}", config.program.display()),
    };
    let stderr = collect_stderr(&mut child);

    let stream = tokio::select! {
        accepted = tokio::time::timeout(CONNECT_TIMEOUT, listener.accept()) => match accepted {
            Ok(Ok(stream)) => stream,
            Ok(Err(err)) => return format!("core channel: {err}"),
            Err(_) => return format!("the core did not connect in {CONNECT_TIMEOUT:?}"),
        },
        exit = child.wait() => return exited(exit, &stderr),
        _ = stopped(stop) => {
            let _ = child.kill().await;
            return "stopped".to_string();
        }
    };

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

    let handshake: Result<(), String> = tokio::select! {
        reply = tokio::time::timeout(CALL_TIMEOUT, hello_rx) => match reply {
            Ok(Ok(Ok(_))) => Ok(()),
            Ok(Ok(Err(err))) => Err(format!("the core refused the handshake: {}", err.message)),
            _ => Err("the core did not answer the handshake".to_string()),
        },
        exit = child.wait() => Err(exited(exit, &stderr)),
        _ = stopped(stop) => Err("stopped".to_string()),
    };
    let reason = match handshake {
        Err(reason) => reason,
        Ok(()) => {
            *shared.link.lock().unwrap() = Some(Link {
                tx,
                pending: pending.clone(),
            });
            shared.set_status(CoreState::Ready, None);
            tokio::select! {
                exit = child.wait() => exited(exit, &stderr),
                _ = &mut reader_task => "the core closed its channel".to_string(),
                _ = stopped(stop) => "stopped".to_string(),
            }
        }
    };

    shared.link.lock().unwrap().take();
    for (_, waiter) in pending.lock().unwrap().drain() {
        let _ = waiter.send(Err(CoreError::local(CODE_NOT_RUNNING, "the core stopped")));
    }
    // Closing both halves closes the channel, and a core that sees it closed saves its data and exits.
    writer_task.abort();
    reader_task.abort();
    if tokio::time::timeout(EXIT_GRACE, child.wait())
        .await
        .is_err()
    {
        let _ = child.kill().await;
    }
    reason
}

fn spawn(config: &CoreConfig, channel: &str, token: &str) -> std::io::Result<Child> {
    let mut command = Command::new(&config.program);
    command
        .env("POROS_CORE_PIPE", channel)
        .env("POROS_CORE_TOKEN", token)
        .env("POROS_DATA_DIR", &config.data_dir)
        .env("POROS_VERSION", env!("CARGO_PKG_VERSION"))
        .stdin(std::process::Stdio::null())
        .stdout(std::process::Stdio::null())
        .stderr(std::process::Stdio::piped())
        .kill_on_drop(true);
    #[cfg(windows)]
    {
        const CREATE_NO_WINDOW: u32 = 0x0800_0000;
        command.creation_flags(CREATE_NO_WINDOW);
    }
    command.spawn()
}

fn collect_stderr(child: &mut Child) -> Arc<Mutex<VecDeque<String>>> {
    let tail = Arc::new(Mutex::new(VecDeque::with_capacity(STDERR_TAIL)));
    if let Some(stderr) = child.stderr.take() {
        let tail = tail.clone();
        tauri::async_runtime::spawn(async move {
            let mut lines = BufReader::new(stderr).lines();
            while let Ok(Some(line)) = lines.next_line().await {
                let mut tail = tail.lock().unwrap();
                if tail.len() == STDERR_TAIL {
                    tail.pop_front();
                }
                tail.push_back(line);
            }
        });
    }
    tail
}

fn exited(
    exit: std::io::Result<std::process::ExitStatus>,
    stderr: &Mutex<VecDeque<String>>,
) -> String {
    let how = match exit {
        Ok(status) => format!("the core exited with {status}"),
        Err(err) => format!("the core is gone: {err}"),
    };
    let tail = stderr.lock().unwrap();
    if tail.is_empty() {
        how
    } else {
        format!("{how}: {}", Vec::from(tail.clone()).join("\n"))
    }
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

    pub struct Listener(NamedPipeServer);

    impl Listener {
        /// A single instance that must be the first with this name, so nothing can sit in front of it.
        pub fn bind(name: &str) -> std::io::Result<Self> {
            ServerOptions::new()
                .first_pipe_instance(true)
                .max_instances(1)
                .reject_remote_clients(true)
                .create(name)
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

    /// Talks to a real core. Set POROS_TEST_CORE to the built PorosCore.exe to run it.
    #[tokio::test(flavor = "multi_thread")]
    async fn calls_the_core_and_receives_its_events() {
        let Some(program) = std::env::var_os("POROS_TEST_CORE") else {
            eprintln!("POROS_TEST_CORE is not set, skipping");
            return;
        };
        let data_dir =
            std::env::temp_dir().join(format!("poros-core-test-{}", random_hex(4).unwrap()));
        let (tx, mut events) = mpsc::unbounded_channel();
        let core = Core::start(
            CoreConfig {
                program: program.into(),
                data_dir: data_dir.clone(),
            },
            Channel(tx),
        );

        let deadline = Instant::now() + CONNECT_TIMEOUT;
        while core.status().state != CoreState::Ready {
            assert!(
                Instant::now() < deadline,
                "core did not become ready: {:?}",
                core.status()
            );
            tokio::time::sleep(Duration::from_millis(50)).await;
        }

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

        core.shutdown().await;
        assert_eq!(core.status().state, CoreState::Down);
        let error = core
            .call("app.info", Value::Null)
            .await
            .expect_err("a stopped core");
        assert_eq!(error.code, CODE_NOT_RUNNING);
        std::fs::remove_dir_all(&data_dir).expect("the stopped core must release its data folder");
    }
}
