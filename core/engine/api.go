package engine

// The types below are the contract with the interface: tygo turns them into src/platform/core-types.ts.

// AppInfo describes the running core.
type AppInfo struct {
	SingBox string `json:"singBox"`
	Xray    string `json:"xray"`
	DataDir string `json:"dataDir"`
	// Elevated is a core with the administrator rights TUN needs.
	Elevated bool `json:"elevated"`
}

// Settings are the user's choices the core keeps between runs.
type Settings struct {
	// UIMode is "simple" or "advanced".
	UIMode string `json:"uiMode"`
	// Language is "" to follow the system, or "en" or "ru".
	Language       string `json:"language"`
	OnboardingDone bool   `json:"onboardingDone"`
	// Tun and SystemProxy are independent: both, either or neither can be on.
	Tun         bool `json:"tun"`
	SystemProxy bool `json:"systemProxy"`
	// Server is the ID of the server to connect to, "" before one is chosen.
	Server string `json:"server"`
	// Preset is the ID of the routing preset in use.
	Preset   string `json:"preset"`
	CheckIP  bool   `json:"checkIp"`
	AllowLAN bool   `json:"allowLan"`
	// IPv6 lets connections use IPv6; off, names resolve to IPv4 only and IPv6 connections are refused.
	IPv6 bool `json:"ipv6"`
	// RealityOnXray runs VLESS with REALITY over TCP and gRPC on Xray rather than sing-box.
	RealityOnXray bool `json:"realityOnXray"`
	// SubscriptionAutoUpdate updates the subscriptions on a schedule, every SubscriptionUpdateHours hours; 0 hours
	// is what each provider advises, or 6 hours.
	SubscriptionAutoUpdate  bool `json:"subscriptionAutoUpdate"`
	SubscriptionUpdateHours int  `json:"subscriptionUpdateHours"`
	// SubscriptionViaProxy downloads subscriptions through the running connection, for providers whose site is
	// blocked. Without a connection they download directly.
	SubscriptionViaProxy bool `json:"subscriptionViaProxy"`
	// SendDeviceInfo sends providers a device ID and the system's version, for those that limit devices.
	SendDeviceInfo bool `json:"sendDeviceInfo"`
	// ShowChanges shows what a subscription update by hand added and removed.
	ShowChanges bool `json:"showChanges"`
	// AutoConnect connects to the chosen server when Poros starts.
	AutoConnect bool `json:"autoConnect"`
	// Autostart, StartMinimized and CloseToTray are for the shell: start with Windows, start in the tray, hide to
	// the tray on close. poros:// links have no setting: the shell reads and changes what Windows holds.
	Autostart      bool `json:"autostart"`
	StartMinimized bool `json:"startMinimized"`
	CloseToTray    bool `json:"closeToTray"`
	// KillSwitch is off, connected or always. Connected blocks the internet from a Start until the Stop, through
	// a broken connection and a failed core; always blocks it while Poros runs. Both need administrator rights.
	KillSwitch string `json:"killSwitch"`
	// TextSize is "normal" or "large".
	TextSize string `json:"textSize"`
	// UpdateCheck looks for a new Poros at launch and every 4 hours and downloads it; it installs only when the
	// user asks.
	UpdateCheck bool `json:"updateCheck"`
	// DebugLog adds every connection and more of what the core does to the log; errors are always there.
	DebugLog bool `json:"debugLog"`
}

// Subscription is a link from a provider and what its last update brought.
type Subscription struct {
	ID string `json:"id"`
	// Name is the user's name for it, else the provider's title, else the host of the URL.
	Name string `json:"name"`
	URL  string `json:"url"`
	// AddedAt, UpdatedAt and Expire are Unix milliseconds. UpdatedAt is the last update that worked, Expire 0 is no
	// end date.
	AddedAt   int64 `json:"addedAt"`
	UpdatedAt int64 `json:"updatedAt"`
	// Servers counts the servers it holds; Skipped the items of the last update that did not become servers.
	Servers int `json:"servers"`
	Skipped int `json:"skipped"`
	// Upload and Download are the bytes used, Total the quota; Total 0 is no quota.
	Upload     int64  `json:"upload"`
	Download   int64  `json:"download"`
	Total      int64  `json:"total"`
	Expire     int64  `json:"expire"`
	WebPageURL string `json:"webPageUrl"`
	SupportURL string `json:"supportUrl"`
	Announce   string `json:"announce"`
	// Error is the reason the last update failed, "" when it worked; ErrorText tells more, in English.
	Error     string `json:"error"`
	ErrorText string `json:"errorText"`
}

// SubscriptionUpdate is what adding or updating a subscription did.
type SubscriptionUpdate struct {
	Subscription Subscription `json:"subscription"`
	// Added and Removed count the servers that came and went, and name up to 50 of each; Changed counts servers
	// that stayed with new settings.
	Added        int      `json:"added"`
	Removed      int      `json:"removed"`
	Changed      int      `json:"changed"`
	AddedNames   []string `json:"addedNames"`
	RemovedNames []string `json:"removedNames"`
	// Skips groups the skipped items by reason.
	Skips []SkipReason `json:"skips"`
}

// SkipReason is why some items of a subscription did not become servers.
type SkipReason struct {
	// Reason is in English. Unsupported is a server Poros cannot run, as opposed to a broken one.
	Reason      string `json:"reason"`
	Unsupported bool   `json:"unsupported"`
	Count       int    `json:"count"`
	// Names are up to three of the items.
	Names []string `json:"names"`
}

// Server is one server of a subscription.
type Server struct {
	ID             string `json:"id"`
	SubscriptionID string `json:"subscriptionId"`
	Name           string `json:"name"`
	// Protocol is vless, vmess, trojan, shadowsocks, hysteria2, tuic, socks, http or anytls.
	Protocol string `json:"protocol"`
	Host     string `json:"host"`
	Port     int    `json:"port"`
	// Country is the ISO 3166 code of a flag in the name, "" without one.
	Country string `json:"country"`
	// Latency is the last delay test in milliseconds: 0 before a test, -1 when the server did not answer.
	Latency int `json:"latency"`
}

// ServerLatency is a delay test result: Latency in milliseconds, -1 when the server did not answer.
type ServerLatency struct {
	ServerID string `json:"serverId"`
	Latency  int    `json:"latency"`
}

// ServersTested brings delay test results as they come; Done is the last report of a test. The delay of the server
// in use comes now and then too while the connection runs, without Done.
type ServersTested struct {
	Results []ServerLatency `json:"results"`
	Done    bool            `json:"done"`
}

// Traffic is the traffic through the server: bytes per second over the last second, and in all since the start.
type Traffic struct {
	Upload        int64 `json:"upload"`
	Download      int64 `json:"download"`
	TotalUpload   int64 `json:"totalUpload"`
	TotalDownload int64 `json:"totalDownload"`
}

// Connection states.
const (
	ConnectionIdle     = "idle"
	ConnectionStarting = "starting"
	ConnectionRunning  = "running"
	ConnectionStopping = "stopping"
)

// ConnectionState is the connection as it is.
type ConnectionState struct {
	// State is idle, starting, running or stopping.
	State string `json:"state"`
	// ServerID, Tun and SystemProxy are what the connection runs with; the settings may already say otherwise.
	ServerID    string `json:"serverId"`
	Tun         bool   `json:"tun"`
	SystemProxy bool   `json:"systemProxy"`
	// Since is when the connection came up, Unix milliseconds.
	Since int64 `json:"since"`
	// Error is the reason the last start failed or a running connection lost a part, such as system_proxy_failed;
	// ErrorText tells more, in English.
	Error     string `json:"error"`
	ErrorText string `json:"errorText"`
}

// Kill switch modes.
const (
	KillSwitchOff       = "off"
	KillSwitchConnected = "connected"
	KillSwitchAlways    = "always"
)

// KillSwitchState is the block of the kill switch.
type KillSwitchState struct {
	// On is the block: only Poros and its TUN reach the network, everything else waits for the connection.
	On bool `json:"on"`
	// Error is why the block is off although the setting asks for it: kill_switch_needs_elevation, or
	// kill_switch_failed with ErrorText in English.
	Error     string `json:"error"`
	ErrorText string `json:"errorText"`
}

// Update states.
const (
	UpdateIdle        = "idle"
	UpdateChecking    = "checking"
	UpdateDownloading = "downloading"
	UpdateReady       = "ready"
)

// Reasons of an install that failed, as the updater gives them.
const (
	// InstallBusy is a file of Poros that could not be replaced, most likely held by another program.
	InstallBusy = "busy"
	// InstallDamaged is an archive that is no release the updater installs.
	InstallDamaged = "damaged"
	InstallFailed  = "failed"
)

// UpdateState is what Poros knows of its own updates.
type UpdateState struct {
	// Available is a build that updates itself: one with a version and updater.exe next to it.
	Available bool   `json:"available"`
	Current   string `json:"current"`
	// State is idle, checking, downloading or ready; Version is the release downloading or ready to install.
	State   string `json:"state"`
	Version string `json:"version"`
	// CheckedAt is the last check that worked, Unix milliseconds, 0 before one.
	CheckedAt int64 `json:"checkedAt"`
	// Error is update_failed when the last check or download did not work; ErrorText tells why, in English.
	Error     string `json:"error"`
	ErrorText string `json:"errorText"`
	// InstallError is why the install of InstallVersion failed before this start: busy, damaged or failed. The
	// updater put the old files back, and Poros runs as before.
	InstallError   string `json:"installError"`
	InstallVersion string `json:"installVersion"`
}

// UpdateInstall is the downloaded release, checked once more, for the updater.
type UpdateInstall struct {
	Archive string `json:"archive"`
}

// IPInfo is what outside services know about the addresses of a way out: IP is the IPv4 address, IPv6 the IPv6 one,
// either empty when the way out has none. The place and the provider are those of the IPv4 address when there is one.
type IPInfo struct {
	IP   string `json:"ip"`
	IPv6 string `json:"ipv6"`
	// CountryCode is ISO 3166; the interface names the country.
	CountryCode string `json:"countryCode"`
	City        string `json:"city"`
	ISP         string `json:"isp"`
	// ASN is the provider's autonomous system, 0 when unknown.
	ASN int `json:"asn"`
}

// DNSServer is a server that resolved the names of a leak test.
type DNSServer struct {
	IP          string `json:"ip"`
	CountryCode string `json:"countryCode"`
	City        string `json:"city"`
	ISP         string `json:"isp"`
	ASN         int    `json:"asn"`
	// Home is a server of the user's own provider: the names went past the VPN.
	Home bool `json:"home"`
}

// Leak results.
const (
	// LeakUnknown is no leak test: no connection, or the test failed.
	LeakUnknown = ""
	LeakNone    = "none"
	LeakFound   = "leak"
)

// NetworkState is what outside services saw at the last check.
type NetworkState struct {
	Checking bool `json:"checking"`
	// Direct are the addresses without the VPN; Proxy the ones through it, empty without a connection.
	Direct IPInfo      `json:"direct"`
	Proxy  IPInfo      `json:"proxy"`
	DNS    []DNSServer `json:"dns"`
	// Leak is "", none or leak.
	Leak string `json:"leak"`
	// CheckedAt is Unix milliseconds, 0 before the first check.
	CheckedAt int64 `json:"checkedAt"`
	// Error is check_failed when a part did not work; ErrorText tells which, in English.
	Error     string `json:"error"`
	ErrorText string `json:"errorText"`
}

// Preset is a routing preset: which traffic goes through the server.
type Preset struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Builtin bool   `json:"builtin"`
	// BasedOn is the built-in preset a copy started from, "" for a preset made from scratch, which runs on the
	// standard rules: all traffic through the server. Sites and Apps count what the user's preset adds.
	BasedOn string `json:"basedOn"`
	Sites   int    `json:"sites"`
	Apps    int    `json:"apps"`
}

// Where the traffic of a preset entry goes.
const (
	ActionProxy  = "proxy"
	ActionDirect = "direct"
	ActionBlock  = "block"
)

// PresetEntry is a site, address or subnet, or a program, of a preset copy and where its traffic goes.
type PresetEntry struct {
	// Value is a domain (it takes its subdomains too), an address, a subnet, a program's file name or its full path.
	Value  string `json:"value"`
	Action string `json:"action"`
}

// InheritedRule is a rule a user's preset keeps from its built-in preset or the standard rules; the user's entries
// come first.
type InheritedRule struct {
	// Name is in English; the interface translates the names it knows.
	Name   string `json:"name"`
	Action string `json:"action"`
}

// PresetCopy is a user's preset as the editor shows it. An empty ID is a new preset, an empty BasedOn one made from
// scratch.
type PresetCopy struct {
	ID      string        `json:"id"`
	BasedOn string        `json:"basedOn"`
	Name    string        `json:"name"`
	Sites   []PresetEntry `json:"sites"`
	Apps    []PresetEntry `json:"apps"`
	// DNS is the remote DNS server: "" keeps the preset's, otherwise https://, tls:// or a plain address.
	DNS string `json:"dns"`
	// Default is where the traffic goes that no entry and no inherited rule takes: proxy, direct or block. ""
	// keeps the one of the built-in preset.
	Default   string          `json:"default"`
	Inherited []InheritedRule `json:"inherited"`
}

// PresetEntries are the sites and programs of a user's preset.
type PresetEntries struct {
	Sites []PresetEntry `json:"sites"`
	Apps  []PresetEntry `json:"apps"`
}

// PresetLists are the entries of a preset as the lists window shows them: a text per action (proxy, direct, block)
// with a line per site, address, subnet or program.
type PresetLists struct {
	Lists map[string]string `json:"lists"`
}

// PresetListProblem is a line of the lists window that became no entry.
type PresetListProblem struct {
	// Action names the tab, Line counts from 1 on it, Text is the line without the spaces around it.
	Action string `json:"action"`
	Line   int    `json:"line"`
	Text   string `json:"text"`
	// Reason is invalid_entry for a line that is no site, address, subnet or program, and duplicate for one already
	// on the tab Other.
	Reason string `json:"reason"`
	Other  string `json:"other"`
}

// PresetListsRead is what the lists window holds: the entries of its lines, and the lines that are none.
type PresetListsRead struct {
	Sites    []PresetEntry       `json:"sites"`
	Apps     []PresetEntry       `json:"apps"`
	Problems []PresetListProblem `json:"problems"`
}

// PresetLink is a preset as a poros://route/ link.
type PresetLink struct {
	Link string `json:"link"`
}

// OutsideLink is a link that came from outside Poros, such as a browser.
type OutsideLink struct {
	Link string `json:"link"`
}

// LinkInfo is what a poros:// link from outside asks to add; nothing is added until the user agrees.
type LinkInfo struct {
	// Kind is subscription or preset.
	Kind string `json:"kind"`
	// URL and Name are the subscription a subscription link adds; Name may be empty.
	URL  string `json:"url"`
	Name string `json:"name"`
	// Preset is the copy a preset link adds.
	Preset *PresetCopy `json:"preset,omitempty"`
}

// Kinds of LinkInfo.
const (
	LinkSubscription = "subscription"
	LinkPreset       = "preset"
)

// RenameSubscription gives a subscription the user's name; an empty one goes back to the provider's title.
type RenameSubscription struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// AddSubscription asks to add a subscription. An empty name takes the provider's title or the host of the URL.
type AddSubscription struct {
	URL  string `json:"url"`
	Name string `json:"name"`
}

// ByID names one object.
type ByID struct {
	ID string `json:"id"`
}

// ServersQuery picks the servers of one subscription, or of all with an empty ID.
type ServersQuery struct {
	SubscriptionID string `json:"subscriptionId"`
}

// ServersChanged tells which subscription's servers changed.
type ServersChanged struct {
	SubscriptionID string `json:"subscriptionId"`
}

// Methods the core answers.
const (
	MethodAppInfo             = "app.info"
	MethodSettingsGet         = "settings.get"
	MethodSettingsSet         = "settings.set"
	MethodSubscriptionsList   = "subscriptions.list"
	MethodSubscriptionsAdd    = "subscriptions.add"
	MethodSubscriptionsUpdate = "subscriptions.update"
	MethodSubscriptionsRemove = "subscriptions.remove"
	MethodServersList         = "servers.list"
	MethodPresetsList         = "presets.list"
	// MethodConnectionStart connects with the settings, or reconnects with them: it answers at once with the
	// starting state, and connection.changed tells how it went.
	MethodConnectionStart = "connection.start"
	MethodConnectionStop  = "connection.stop"
	MethodConnectionState = "connection.state"
	// MethodNetworkCheck starts a check of the addresses and DNS at once; network.changed brings the result.
	MethodNetworkCheck = "network.check"
	MethodNetworkState = "network.state"
	// MethodServersTest measures the delay of the servers of a subscription, or of all; servers.tested brings the
	// results.
	MethodServersTest         = "servers.test"
	MethodSubscriptionsRename = "subscriptions.rename"
	// MethodPresetsGet gives a user's preset for the editor; for a built-in preset, a new copy of it, and for "" a
	// new preset from scratch.
	MethodPresetsGet    = "presets.get"
	MethodPresetsSave   = "presets.save"
	MethodPresetsRemove = "presets.remove"
	MethodPresetsExport = "presets.export"
	MethodPresetsImport = "presets.import"
	// MethodPresetsWriteLists gives the entries of a preset as the text of the lists window, MethodPresetsReadLists
	// the entries of that text and the lines it could not read; neither saves anything.
	MethodPresetsWriteLists = "presets.writeLists"
	MethodPresetsReadLists  = "presets.readLists"
	// MethodLinksParse reads a poros:// link without adding anything.
	MethodLinksParse = "links.parse"
	// MethodLogsGet gives the last lines of the core's log.
	MethodLogsGet = "logs.get"
	// MethodKillSwitchState gives the block of the kill switch; the setting turns it on and off, and Start and Stop
	// in the connected mode.
	MethodKillSwitchState = "killSwitch.state"
	// MethodUpdatesCheck looks for a new release and downloads it in the background; it answers at once, and
	// updates.changed tells how it went.
	MethodUpdatesCheck = "updates.check"
	MethodUpdatesState = "updates.state"
	// MethodUpdatesInstall gives the shell the downloaded release for the updater, after checking it again.
	MethodUpdatesInstall = "updates.install"
)

// Events the core sends without a request.
const (
	EventSettingsChanged      = "settings.changed"
	EventSubscriptionsChanged = "subscriptions.changed"
	EventServersChanged       = "servers.changed"
	EventConnectionChanged    = "connection.changed"
	EventNetworkChanged       = "network.changed"
	EventServersTested        = "servers.tested"
	// EventConnectionTraffic comes every second while the connection runs.
	EventConnectionTraffic = "connection.traffic"
	EventPresetsChanged    = "presets.changed"
	EventKillSwitchChanged = "killSwitch.changed"
	EventUpdatesChanged    = "updates.changed"
)

// Reasons in the data of a failed request, and in Subscription.Error, for the interface to translate.
const (
	ReasonInvalidURL    = "invalid_url"
	ReasonDuplicate     = "duplicate"
	ReasonNotFound      = "not_found"
	ReasonUnknownPreset = "unknown_preset"
	ReasonInvalidValue  = "invalid_value"
	// ReasonSingleServer is a link to one server where a subscription was asked for.
	ReasonSingleServer = "single_server"
	// ReasonFetchFailed is a subscription that did not download: no network, an error page, a timeout.
	ReasonFetchFailed = "fetch_failed"
	// ReasonInvalidSubscription is a download that is no subscription format Poros reads.
	ReasonInvalidSubscription = "invalid_subscription"
	// ReasonNoServers is a subscription without a server Poros can run.
	ReasonNoServers = "no_servers"
	// ReasonNoServer is a start before a server is chosen, or after it left its subscription.
	ReasonNoServer = "no_server"
	// ReasonServerUnsupported is a server neither core runs.
	ReasonServerUnsupported = "server_unsupported"
	// ReasonNeedsElevation is TUN in a core without administrator rights.
	ReasonNeedsElevation = "needs_elevation"
	// ReasonKillSwitchNeedsElevation is the kill switch in a core without administrator rights.
	ReasonKillSwitchNeedsElevation = "kill_switch_needs_elevation"
	// ReasonKillSwitchFailed is a block of the kill switch that did not turn on or went away.
	ReasonKillSwitchFailed = "kill_switch_failed"
	// ReasonPortBusy is the port of the mixed inbound held by another program.
	ReasonPortBusy = "port_busy"
	// ReasonStartFailed is a core that refused to start; ErrorText has its words.
	ReasonStartFailed = "start_failed"
	// ReasonSystemProxyFailed is a connection that runs without the system proxy, which could not be set.
	ReasonSystemProxyFailed = "system_proxy_failed"
	// ReasonStopTimeout is a connection that did not close in time; the core has to restart to let go of it.
	ReasonStopTimeout = "stop_timeout"
	// ReasonTunBusy is a TUN adapter of an earlier core that Windows has not removed yet.
	ReasonTunBusy = "tun_busy"
	// ReasonCheckOff is a check asked for while the user has it turned off.
	ReasonCheckOff = "check_off"
	// ReasonCheckFailed is a check of which a part did not work: a service did not answer.
	ReasonCheckFailed = "check_failed"
	// ReasonInvalidEntry is a preset entry that is no domain, address, subnet or program.
	ReasonInvalidEntry = "invalid_entry"
	// ReasonInvalidLink is a preset or poros:// link Poros cannot read.
	ReasonInvalidLink = "invalid_link"
	// ReasonUpdateFailed is a check or download of an update that did not work.
	ReasonUpdateFailed = "update_failed"
	// ReasonUpdateUnavailable is a build that does not update itself: it has no version or no updater.exe.
	ReasonUpdateUnavailable = "update_unavailable"
	// ReasonUpdateNotReady is an install asked for before a release was downloaded, or after its archive changed.
	ReasonUpdateNotReady = "update_not_ready"
	// ReasonPresetNameTaken is a preset saved under the name of another one: the tray and the lists would not tell
	// them apart.
	ReasonPresetNameTaken = "preset_name_taken"
)
