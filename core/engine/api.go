package engine

import "PorosCore/profile"

// The types below are the contract with the interface: tygo turns them into src/platform/core-types.ts, and the
// server model of core/profile into src/platform/profile-types.ts.

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
	// AdminAutostart gives the core administrator rights at sign-in without a UAC prompt: a task of the Task
	// Scheduler starts it, and the shell of the autostart waits for it. Turning it on takes rights; turning it off
	// without them leaves the task to remove itself at the next sign-in.
	AdminAutostart bool `json:"adminAutostart"`
	// RunAsAdmin asks for rights through UAC as Poros starts, so TUN and the kill switch ask nothing later. Only the
	// core gets them, as always.
	RunAsAdmin bool `json:"runAsAdmin"`
	// KillSwitch is off, connected or always. It keeps the traffic of the server from going out directly: after a
	// lost connection the TUN, the system proxy and the local proxy stay up with the preset and a refusal instead
	// of the server. Connected holds it from a lost connection until the Stop, always while Poros runs. With the
	// TUN a block of Windows covers the moments without Poros's TUN, which needs administrator rights.
	KillSwitch string `json:"killSwitch"`
	// TextSize is "normal" or "large".
	TextSize string `json:"textSize"`
	// UpdateCheck looks for a new Poros at launch and every 4 hours and downloads it; it installs only when the
	// user asks.
	UpdateCheck bool `json:"updateCheck"`
	// DebugLog adds every connection and more of what the core does to the log; errors are always there.
	DebugLog bool `json:"debugLog"`
}

// Kinds of groups.
const (
	GroupSubscription = "subscription"
	GroupManual       = "manual"
)

// Group holds servers: a subscription brings them from a provider's link, a group of the user's holds those added by
// hand. Subscriptions come first, each kind in the order it was added.
type Group struct {
	ID string `json:"id"`
	// Kind is subscription or manual.
	Kind string `json:"kind"`
	// Name is the user's name for it; a subscription without one has the provider's title, else the host of the URL.
	Name string `json:"name"`
	// URL is a subscription's link, "" for a group of the user's.
	URL string `json:"url"`
	// AddedAt, UpdatedAt and Expire are Unix milliseconds. UpdatedAt is the last update of a subscription that
	// worked, Expire 0 is no end date.
	AddedAt   int64 `json:"addedAt"`
	UpdatedAt int64 `json:"updatedAt"`
	// Servers counts the servers it holds; Skipped the items of a subscription's last update that did not become
	// servers.
	Servers int `json:"servers"`
	Skipped int `json:"skipped"`
	// The rest is a subscription's, from its provider.
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
	Group Group `json:"group"`
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

// Server is one server of a group.
type Server struct {
	ID      string `json:"id"`
	GroupID string `json:"groupId"`
	Name    string `json:"name"`
	// Protocol is the type of its profile: vless, vmess, trojan, shadowsocks, hysteria2, tuic, socks, http, anytls,
	// wireguard, hysteria, naive, ssh, snell, openvpn, openconnect or masque.
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

// KillSwitchState is what the kill switch holds.
type KillSwitchState struct {
	// On is the kill switch at work: a connection runs under it, or the traffic of the server is refused, or the
	// block of Windows is on.
	On bool `json:"on"`
	// Full is the block of Windows without Poros's TUN, as when the core came back without rights: only Poros and
	// the local network reach the network, not just the traffic of the server waits.
	Full bool `json:"full"`
	// Error is why the kill switch does not hold what the setting asks for: kill_switch_needs_elevation for the
	// TUN, or kill_switch_failed with ErrorText in English.
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
	// Kind is subscription, preset or server.
	Kind string `json:"kind"`
	// URL and Name are the subscription a subscription link adds; Name may be empty.
	URL  string `json:"url"`
	Name string `json:"name"`
	// Preset is the copy a preset link adds.
	Preset *PresetCopy `json:"preset,omitempty"`
	// Server is the server a server link adds, without an ID or a group yet; servers.add takes the link itself.
	Server *Server `json:"server,omitempty"`
}

// Kinds of LinkInfo.
const (
	LinkSubscription = "subscription"
	LinkPreset       = "preset"
	LinkServer       = "server"
)

// RenameGroup gives a group the user's name. An empty one takes a subscription back to the provider's title; a group
// of the user's needs a name.
type RenameGroup struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// AddGroup creates a group of the user's.
type AddGroup struct {
	Name string `json:"name"`
}

// AddServers adds the servers of a text to a group of the user's: share links one per line, base64 of them, Clash
// YAML, sing-box or Xray JSON, poros://add/ links.
type AddServers struct {
	// GroupID is the group to add to; empty, NewGroup names a group of the user's to create for them.
	GroupID  string `json:"groupId"`
	NewGroup string `json:"newGroup"`
	Text     string `json:"text"`
}

// ServersAdded is what adding servers did.
type ServersAdded struct {
	Group Group `json:"group"`
	// IDs are the added servers in the order of the text.
	IDs []string `json:"ids"`
	// Skips groups the items of the text that did not become servers by reason.
	Skips []SkipReason `json:"skips"`
}

// ServerProfile is a server with all of its settings.
type ServerProfile struct {
	Server  Server          `json:"server"`
	Profile profile.Profile `json:"profile"`
}

// SaveServer stores the settings of a server. With an ID they replace those of that server, which a subscription's
// next update replaces in turn with the provider's; without one the server is new and goes to GroupID, a group of
// the user's.
type SaveServer struct {
	ID      string          `json:"id"`
	GroupID string          `json:"groupId"`
	Profile profile.Profile `json:"profile"`
}

// ServerIDs names servers, in the order given.
type ServerIDs struct {
	IDs []string `json:"ids"`
}

// ServerLinks are links of servers, in the order they were asked for.
type ServerLinks struct {
	Links []string `json:"links"`
}

// ServerConfig is a server as the core that runs it reads it, for the user to copy.
type ServerConfig struct {
	// Core is sing-box or xray.
	Core string `json:"core"`
	// JSON is the core's outbound, indented.
	JSON string `json:"json"`
}

// Cores of ServerConfig.
const (
	CoreSingBox = "sing-box"
	CoreXray    = "xray"
)

// MoveServers moves servers to a group of the user's, after those it holds. Servers of a subscription are copied
// instead, as its next update would bring them back.
type MoveServers struct {
	IDs     []string `json:"ids"`
	GroupID string   `json:"groupId"`
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

// ServersQuery picks the servers of one group, or of all with an empty ID.
type ServersQuery struct {
	GroupID string `json:"groupId"`
}

// ServersChanged tells which group's servers changed.
type ServersChanged struct {
	GroupID string `json:"groupId"`
}

// Methods the core answers.
const (
	MethodAppInfo = "app.info"
	// MethodAppHandOver tells the core that the shell replaces it with another core: it leaves the system proxy
	// and what the kill switch holds to the next one.
	MethodAppHandOver         = "app.handOver"
	MethodSettingsGet         = "settings.get"
	MethodSettingsSet         = "settings.set"
	MethodGroupsList          = "groups.list"
	MethodGroupsAdd           = "groups.add"
	MethodGroupsRename        = "groups.rename"
	MethodGroupsRemove        = "groups.remove"
	MethodSubscriptionsAdd    = "subscriptions.add"
	MethodSubscriptionsUpdate = "subscriptions.update"
	MethodServersList         = "servers.list"
	// MethodServersAdd adds the servers of a text to a group of the user's.
	MethodServersAdd = "servers.add"
	// MethodServersGet gives a server with its whole profile, MethodServersSave stores a changed or new one.
	MethodServersGet    = "servers.get"
	MethodServersSave   = "servers.save"
	MethodServersRemove = "servers.remove"
	// MethodServersClone puts a copy of each server right after it and answers with the IDs of the copies.
	MethodServersClone = "servers.clone"
	// MethodServersMove answers with the IDs the servers have in the group they went to.
	MethodServersMove = "servers.move"
	// MethodServersLinks gives the share links of servers: the link of the protocol, which other clients read, or
	// poros://add/ for a server such a link cannot carry whole.
	MethodServersLinks = "servers.links"
	// MethodServersDeepLinks gives servers as poros://add/ links, which carry all of their settings.
	MethodServersDeepLinks = "servers.deepLinks"
	// MethodServersExport gives a server as the outbound of the core that runs it.
	MethodServersExport = "servers.export"
	MethodPresetsList   = "presets.list"
	// MethodConnectionStart connects with the settings, or reconnects with them: it answers at once with the
	// starting state, and connection.changed tells how it went.
	MethodConnectionStart = "connection.start"
	MethodConnectionStop  = "connection.stop"
	MethodConnectionState = "connection.state"
	// MethodNetworkCheck starts a check of the addresses and DNS at once; network.changed brings the result.
	MethodNetworkCheck = "network.check"
	MethodNetworkState = "network.state"
	// MethodServersTest measures the delay of the servers of a group, or of all; servers.tested brings the results.
	MethodServersTest = "servers.test"
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
	// MethodKillSwitchState gives what the kill switch holds; the setting, Start and Stop change it.
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
	EventSettingsChanged   = "settings.changed"
	EventGroupsChanged     = "groups.changed"
	EventServersChanged    = "servers.changed"
	EventConnectionChanged = "connection.changed"
	EventNetworkChanged    = "network.changed"
	EventServersTested     = "servers.tested"
	// EventConnectionTraffic comes every second while the connection runs.
	EventConnectionTraffic = "connection.traffic"
	EventPresetsChanged    = "presets.changed"
	EventKillSwitchChanged = "killSwitch.changed"
	EventUpdatesChanged    = "updates.changed"
)

// Reasons in the data of a failed request, and in Group.Error, for the interface to translate.
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
	// ReasonNoServers is a subscription, or a text to add servers from, without a server Poros can run.
	ReasonNoServers = "no_servers"
	// ReasonSubscriptionURL is a subscription's link where servers were to be added: it is added as a subscription.
	ReasonSubscriptionURL = "subscription_url"
	// ReasonNotManual is a subscription where a group of the user's was asked for: servers are added and moved only
	// there.
	ReasonNotManual = "not_manual"
	// ReasonNoServer is a start before a server is chosen, or after it left its group.
	ReasonNoServer = "no_server"
	// ReasonServerUnsupported is a server neither core runs.
	ReasonServerUnsupported = "server_unsupported"
	// ReasonNeedsElevation is TUN in a core without administrator rights.
	ReasonNeedsElevation = "needs_elevation"
	// ReasonKillSwitchNeedsElevation is the kill switch in a core without administrator rights.
	ReasonKillSwitchNeedsElevation = "kill_switch_needs_elevation"
	// ReasonLogonTaskNeedsElevation is rights at sign-in asked of a core without administrator rights.
	ReasonLogonTaskNeedsElevation = "logon_task_needs_elevation"
	// ReasonLogonTaskFailed is a sign-in task Windows did not create.
	ReasonLogonTaskFailed = "logon_task_failed"
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
