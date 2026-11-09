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
	// SubscriptionUserAgent replaces Poros's User-Agent when subscriptions download, for providers that serve a
	// format by it; "" keeps Poros's.
	SubscriptionUserAgent string `json:"subscriptionUserAgent"`
	// SubscriptionTLS is the TLS version subscriptions download with: "" for any, "1.2" or "1.3".
	// SubscriptionHTTP is "" for any HTTP version or "1.1" for HTTP/1.1 alone.
	SubscriptionTLS  string `json:"subscriptionTls"`
	SubscriptionHTTP string `json:"subscriptionHttp"`
	// SpeedTestMode is what a speed test measures: "full" for download and upload, "download", "upload",
	// "simple" for one download of SpeedTestURL, or "country" for the country of the test's server alone.
	SpeedTestMode string `json:"speedTestMode"`
	SpeedTestURL  string `json:"speedTestUrl"`
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
	Error     string        `json:"error"`
	ErrorText string        `json:"errorText"`
	Settings  GroupSettings `json:"settings"`
}

// GroupSettings change how the servers of a group connect and, for a subscription, how it downloads and updates.
type GroupSettings struct {
	// ServerAddress, when set, is dialled for every server of the group, such as an IP of a CDN in front of them;
	// each server keeps its own name as the TLS server name and the HTTP host.
	ServerAddress string `json:"serverAddress"`
	// FrontProxy is a server of another group that every server of this one is dialled through; LandingProxy one
	// the traffic leaves by after it. IDs, "" for none.
	FrontProxy   string `json:"frontProxy"`
	LandingProxy string `json:"landingProxy"`

	// The rest is a subscription's. The request: "" takes what the settings of all subscriptions say.
	UserAgent string `json:"userAgent"`
	// TLS is "", "auto", "1.2" or "1.3"; HTTP is "", "auto" or "1.1".
	TLS  string `json:"tls"`
	HTTP string `json:"http"`
	// DeviceInfo is "", "on" or "off", whether the provider is told the device; DeviceID, OS, OSVersion and
	// DeviceModel replace what Poros tells it.
	DeviceInfo  string `json:"deviceInfo"`
	DeviceID    string `json:"deviceId"`
	OS          string `json:"os"`
	OSVersion   string `json:"osVersion"`
	DeviceModel string `json:"deviceModel"`
	// Update is "" as the settings of all subscriptions say, "provider" as the provider advises, "hours" every
	// UpdateHours hours, or "never" for updates by hand only.
	Update      string `json:"update"`
	UpdateHours int    `json:"updateHours"`
	// What an update does after the download. KeepWorking keeps the servers the provider dropped whose last delay
	// test worked; RemoveDuplicates and RemoveInsecure leave out servers the subscription brings twice or that
	// protect the traffic poorly. Test measures the servers afterwards, and by its results RemoveUnavailable
	// removes those that did not answer and SortByLatency puts the fastest first.
	KeepWorking       bool `json:"keepWorking"`
	RemoveDuplicates  bool `json:"removeDuplicates"`
	RemoveInsecure    bool `json:"removeInsecure"`
	Test              bool `json:"test"`
	RemoveUnavailable bool `json:"removeUnavailable"`
	SortByLatency     bool `json:"sortByLatency"`
}

// Choices of GroupSettings.Update.
const (
	GroupUpdateDefault  = ""
	GroupUpdateProvider = "provider"
	GroupUpdateHours    = "hours"
	GroupUpdateNever    = "never"
)

// ConfigureGroup changes the settings of a group and, for a subscription, its URL; an empty URL keeps it.
type ConfigureGroup struct {
	ID       string        `json:"id"`
	URL      string        `json:"url"`
	Settings GroupSettings `json:"settings"`
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
	// wireguard, hysteria, naive, ssh, snell, openvpn, openconnect, masque, chain, custom or direct. A chain, Direct and
	// a custom outbound have no host or port.
	Protocol string `json:"protocol"`
	Host     string `json:"host"`
	Port     int    `json:"port"`
	// Country is the ISO 3166 code of a flag in the name, "" without one.
	Country string `json:"country"`
	// Latency is the last delay test in milliseconds: 0 before a test, -1 when the server did not answer.
	Latency int `json:"latency"`
	// Security is "" for a server that protects the traffic, "weak" for an old cipher or TLS that checks no
	// certificate, "none" for plain text; a server on a private address is never insecure.
	Security string `json:"security"`
	// Upload and Download count the bytes that went through it since it came or its traffic was reset.
	Upload   int64 `json:"upload"`
	Download int64 `json:"download"`
	// SpeedDown and SpeedUp are the last speed test, "" before one; ExitIP and ExitCountry the last IP test.
	SpeedDown   string `json:"speedDown"`
	SpeedUp     string `json:"speedUp"`
	ExitIP      string `json:"exitIp"`
	ExitCountry string `json:"exitCountry"`
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
	// Servers are all the servers the connection goes through: the chosen one, the links of a chain, the proxies
	// of its group and the endpoints of the preset. A change of any of them waits for a restart.
	Servers []string `json:"servers,omitempty"`
	// Since is when the connection came up, Unix milliseconds.
	Since int64 `json:"since"`
	// Error is the reason the last start failed or a running connection lost a part, such as system_proxy_failed;
	// ErrorText tells more, in English.
	Error     string `json:"error"`
	ErrorText string `json:"errorText"`
}

// Modes of a speed test.
const (
	SpeedTestFull     = "full"
	SpeedTestDownload = "download"
	SpeedTestUpload   = "upload"
	SpeedTestSimple   = "simple"
	SpeedTestCountry  = "country"
)

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

// / Preset is a routing preset: which traffic goes through the server.
type Preset struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Builtin bool   `json:"builtin"`
	// BasedOn is the built-in preset a copy started from, "" for a preset made from scratch, which runs on the
	// standard rules: all traffic through the server.
	BasedOn string `json:"basedOn"`
	// Advanced is a preset of the advanced kind: more kinds of conditions, rules of several conditions, a direct DNS.
	Advanced bool `json:"advanced"`
	// Rules counts the user's rules of a copy, and all the rules of a built-in preset.
	Rules int `json:"rules"`
	// Endpoints counts the endpoints of an advanced preset, which a link to it leaves out.
	Endpoints int `json:"endpoints"`
}

// Where the traffic of a preset rule goes. ActionEndpoint is an endpoint of an advanced preset, the one the rule's
// Endpoint names.
const (
	ActionProxy    = "proxy"
	ActionDirect   = "direct"
	ActionBlock    = "block"
	ActionEndpoint = "endpoint"
)

// Kinds of a condition of a preset rule. They are the prefixes of the lists window too, spelled as in Throne.
const (
	// CondSuffix is a domain with its subdomains.
	CondSuffix = "suffix"
	// CondIP is an address or a subnet of the destination.
	CondIP = "ip"
	// CondProcessName is the file name of a program, CondProcessPath its full path.
	CondProcessName = "processName"
	CondProcessPath = "processPath"
	// CondDomain and the kinds below belong to the advanced preset. CondDomain is a domain without its subdomains.
	CondDomain           = "domain"
	CondKeyword          = "keyword"
	CondRegex            = "regex"
	CondProcessPathRegex = "processPathRegex"
	// CondPort is a port or a range, 8000-8080.
	CondPort = "port"
	// CondProtocol is a protocol sing-box recognizes: tls, http, quic, dns, stun, bittorrent, dtls, ssh, rdp, ntp.
	CondProtocol = "protocol"
	// CondNetwork is tcp or udp, CondIPVersion 4 or 6.
	CondNetwork   = "network"
	CondIPVersion = "ipVersion"
	// CondSourceIP is an address or a subnet of the source, as of a device in the local network.
	CondSourceIP = "sourceIP"
	// CondPrivate is the private networks; it takes no values and has no line in the lists window.
	CondPrivate = "private"
)

// RuleCondition is a condition of a preset rule: it matches by any of its values.
type RuleCondition struct {
	Kind   string   `json:"kind"`
	Values []string `json:"values"`
}

// PresetRule is a rule of a user's preset: it matches when every condition does. A rule of a simple preset has one
// condition of a simple kind with one value.
type PresetRule struct {
	// Name is the rule's own name in an advanced preset, "" for none.
	Name       string          `json:"name"`
	Conditions []RuleCondition `json:"conditions"`
	// Invert takes what the conditions do not match; Off keeps the rule out of the connection. Both are advanced.
	Invert bool   `json:"invert"`
	Off    bool   `json:"off"`
	Action string `json:"action"`
	// Endpoint is the ID of the profile of the endpoint ActionEndpoint sends to, one of the preset's Endpoints; ""
	// for the other actions.
	Endpoint string `json:"endpoint"`
}

// RuleProblem is the data of an error about a rule of a preset: which rule, condition and value.
type RuleProblem struct {
	// Reason is invalid_entry for a value that is not of its kind, invalid_value for an unknown kind or action,
	// no_condition, empty_condition for a condition without values, advanced_only for what a simple preset cannot
	// have, and duplicate for a rule that repeats the rule Other.
	Reason string `json:"reason"`
	// Rule counts the rules from 0, Line the values of the condition of Kind from 1.
	Rule  int    `json:"rule"`
	Kind  string `json:"kind"`
	Line  int    `json:"line"`
	Value string `json:"value"`
	Other int    `json:"other"`
}

// InheritedRule is a rule a user's preset keeps from its built-in preset or the standard rules.
type InheritedRule struct {
	// Name is in English; the interface translates the names it knows.
	Name string `json:"name"`
	// Action is proxy, direct or block, and dns for the rule of DNS queries, which Poros answers.
	Action string `json:"action"`
	// First is a rule before the user's rules; the others come after them.
	First bool `json:"first"`
}

// PresetCopy is a preset as the editor shows it. An empty ID is a new preset, an empty BasedOn one made from
// scratch. A built-in preset comes with Builtin set and no rules of its own; it is never saved.
type PresetCopy struct {
	ID       string       `json:"id"`
	Builtin  bool         `json:"builtin"`
	BasedOn  string       `json:"basedOn"`
	Name     string       `json:"name"`
	Advanced bool         `json:"advanced"`
	Rules    []PresetRule `json:"rules"`
	// DNS is the remote DNS server: "" is the default one, otherwise https://, tls:// or a plain address.
	DNS string `json:"dns"`
	// DirectDNS is the DNS server of direct traffic of an advanced preset: "" is the default one, local the
	// system's, otherwise https://, tls:// or a plain address.
	DirectDNS string `json:"directDns"`
	// Default is where the traffic goes that no rule takes: proxy, direct or block. "" keeps the one of the
	// built-in preset.
	Default   string          `json:"default"`
	Inherited []InheritedRule `json:"inherited"`
	// Endpoints of an advanced preset are the IDs of OpenVPN and OpenConnect profiles of any group. They connect
	// alongside the server, the networks they announce go through them, and rules send them more. They are this
	// computer's profiles: a link to the preset leaves them out, and the rules to them.
	Endpoints []string `json:"endpoints"`
}

// DNSLocal is the system's DNS as the direct DNS of a preset.
const DNSLocal = "local"

// PresetRules are the rules of a preset of a kind: presets.check gives them back checked, with every value as the
// core reads it and narrower rules over wider ones. Endpoints are those of the preset, which its rules may send to.
type PresetRules struct {
	Advanced  bool         `json:"advanced"`
	Rules     []PresetRule `json:"rules"`
	Endpoints []string     `json:"endpoints,omitempty"`
}

// PresetLists are the rules of a preset as the lists window shows them: a text per action (proxy, direct, block)
// with a line per rule, each starting with the prefix of its kind.
type PresetLists struct {
	Lists map[string]string `json:"lists"`
}

// PresetListsInput is the text of the lists window and the rules it was written from.
type PresetListsInput struct {
	Advanced bool              `json:"advanced"`
	Rules    []PresetRule      `json:"rules"`
	Lists    map[string]string `json:"lists"`
}

// PresetListProblem is a line of the lists window that became no rule.
type PresetListProblem struct {
	// Action names the tab, Line counts from 1 on it, Text is the line without the spaces around it.
	Action string `json:"action"`
	Line   int    `json:"line"`
	Text   string `json:"text"`
	// Reason is no_prefix, advanced_only for a kind of the advanced preset in a simple one, invalid_entry for a value
	// that is not of its kind, and duplicate for a line already on the tab Other, or of a rule to an endpoint when
	// Other is endpoint.
	Reason string `json:"reason"`
	Other  string `json:"other"`
}

// PresetListsRead is what the lists window holds: the rules with its lines, and the lines that are none. The rules
// the window has a line for keep their places, those without one stay, and new lines come last; then narrower rules
// rise over wider ones.
type PresetListsRead struct {
	Rules    []PresetRule        `json:"rules"`
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
	// Profile is its whole profile, for the editor to change before it is added.
	Server  *Server          `json:"server,omitempty"`
	Profile *profile.Profile `json:"profile,omitempty"`
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

// ServerForms is what the profile editor draws: the form of each type of profile and the choices of «New profile».
type ServerForms struct {
	Forms []profile.ProfileForm `json:"forms"`
	New   []profile.NewProfile  `json:"new"`
}

// ServerCheck is what the editor tells of a profile before it is saved.
type ServerCheck struct {
	// Core is the core that would run it with the current settings: sing-box or xray.
	Core string `json:"core"`
	// Problem is why it would not run, as the core says it, in English; empty when it would.
	Problem string `json:"problem"`
	// Link is its share link once it would run; a chain has none.
	Link string `json:"link"`
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

// ServersQuery picks servers: those IDs names, else those of one group, or of all with an empty GroupID.
type ServersQuery struct {
	GroupID string   `json:"groupId"`
	IDs     []string `json:"ids,omitempty"`
}

// ServerSpeed is a speed test of a server: the rates as the test writes them, such as "84.2 Mbps", "" for a
// direction the mode leaves out, and the country of the test's server; Error tells why it failed, in English.
type ServerSpeed struct {
	ServerID string `json:"serverId"`
	Download string `json:"download"`
	Upload   string `json:"upload"`
	Country  string `json:"country"`
	Error    string `json:"error"`
}

// ServersSpeedTested brings speed test results as they come; Done is the last report of a test.
type ServersSpeedTested struct {
	Results []ServerSpeed `json:"results"`
	Done    bool          `json:"done"`
}

// ServerIP is the address sites see a server's traffic come from, with its country; Error tells why the test failed.
type ServerIP struct {
	ServerID string `json:"serverId"`
	IP       string `json:"ip"`
	Country  string `json:"country"`
	Error    string `json:"error"`
}

// ServersIPTested brings IP test results; Done is the last report of a test.
type ServersIPTested struct {
	Results []ServerIP `json:"results"`
	Done    bool       `json:"done"`
}

// ServersResolved counts the servers whose host names were replaced by the addresses they resolve to.
type ServersResolved struct {
	Resolved int `json:"resolved"`
}

// CleanServers removes servers of a group, or of all: those that did not answer their last delay test, the second
// and later of servers that are the same but for the name, and those that protect the traffic poorly.
type CleanServers struct {
	GroupID     string `json:"groupId"`
	Unavailable bool   `json:"unavailable"`
	Duplicates  bool   `json:"duplicates"`
	Insecure    bool   `json:"insecure"`
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
	MethodAppHandOver  = "app.handOver"
	MethodSettingsGet  = "settings.get"
	MethodSettingsSet  = "settings.set"
	MethodGroupsList   = "groups.list"
	MethodGroupsAdd    = "groups.add"
	MethodGroupsRename = "groups.rename"
	MethodGroupsRemove = "groups.remove"
	// MethodGroupsConfigure changes the settings of a group and the URL of a subscription.
	MethodGroupsConfigure     = "groups.configure"
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
	// MethodServersForms gives what the profile editor draws.
	MethodServersForms = "servers.forms"
	// MethodServersCheck tells, of a profile not saved yet, which core would run it, why it would not run, and its
	// share link; it takes servers.save's parameters.
	MethodServersCheck = "servers.check"
	MethodPresetsList  = "presets.list"
	// MethodConnectionStart connects with the settings, or reconnects with them: it answers at once with the
	// starting state, and connection.changed tells how it went.
	MethodConnectionStart = "connection.start"
	MethodConnectionStop  = "connection.stop"
	MethodConnectionState = "connection.state"
	// MethodNetworkCheck starts a check of the addresses and DNS at once; network.changed brings the result.
	MethodNetworkCheck = "network.check"
	MethodNetworkState = "network.state"
	// MethodServersTest measures the delay of servers; servers.tested brings the results.
	MethodServersTest = "servers.test"
	// MethodServersSpeedTest measures the speed of servers one by one, as speedTestMode says; servers.speedTested
	// brings the results.
	MethodServersSpeedTest = "servers.speedTest"
	// MethodServersIPTest finds the address sites see each server's traffic come from; servers.ipTested brings it.
	MethodServersIPTest = "servers.ipTest"
	// MethodServersResolve replaces the host names of servers by the addresses they resolve to, as Throne does;
	// TLS and HTTP keep the names.
	MethodServersResolve = "servers.resolve"
	// MethodServersClearResults forgets the delay, speed and IP tests of servers; MethodServersStopTests stops the
	// tests that run.
	MethodServersClearResults = "servers.clearResults"
	MethodServersStopTests    = "servers.stopTests"
	MethodServersResetTraffic = "servers.resetTraffic"
	// MethodServersClean removes servers as CleanServers says and answers with their IDs.
	MethodServersClean = "servers.clean"
	// MethodPresetsGet gives a preset for the editor; a built-in one comes with Builtin set, to show and to copy.
	MethodPresetsGet = "presets.get"
	// MethodPresetsSave stores a user's preset, a new one when it has no ID; its rules are checked as by
	// MethodPresetsCheck.
	MethodPresetsSave   = "presets.save"
	MethodPresetsRemove = "presets.remove"
	MethodPresetsExport = "presets.export"
	MethodPresetsImport = "presets.import"
	// MethodPresetsCheck checks the rules of a preset without saving them; a wrong one fails with a RuleProblem as
	// the data of the error.
	MethodPresetsCheck = "presets.check"
	// MethodPresetsWriteLists gives the rules of a preset as the text of the lists window, MethodPresetsReadLists
	// the rules with that text and the lines it could not read; neither saves anything.
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
	EventSettingsChanged    = "settings.changed"
	EventGroupsChanged      = "groups.changed"
	EventServersChanged     = "servers.changed"
	EventConnectionChanged  = "connection.changed"
	EventNetworkChanged     = "network.changed"
	EventServersTested      = "servers.tested"
	EventServersSpeedTested = "servers.speedTested"
	EventServersIPTested    = "servers.ipTested"
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
	// ReasonPortPoros is the port of the mixed inbound held by another Poros on this computer, as of another Windows
	// user.
	ReasonPortPoros = "port_poros"
	// ReasonTunTaken is a TUN asked for while another Poros on this computer has its own on: Windows has one set of
	// routes for every user.
	ReasonTunTaken = "tun_taken"
	// ReasonKillSwitchTaken is a block of the kill switch asked for while another Poros on this computer holds one.
	ReasonKillSwitchTaken = "kill_switch_taken"
	// ReasonStartFailed is a core that refused to start; ErrorText has its words.
	ReasonStartFailed = "start_failed"
	// ReasonEndpointFailed is an endpoint of the preset that did not connect, or whose profile is gone or of another
	// type now; ErrorText names it.
	ReasonEndpointFailed = "endpoint_failed"
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
	// ReasonInvalidEntry is a value of a preset rule that is not of its kind: no site, address, port or program.
	ReasonInvalidEntry = "invalid_entry"
	// ReasonNoPrefix is a line of the lists window without the prefix of its kind.
	ReasonNoPrefix = "no_prefix"
	// ReasonAdvancedOnly is what only an advanced preset has, found in a simple one: a kind, several conditions or
	// values, an inverted or switched-off rule, a direct DNS.
	ReasonAdvancedOnly = "advanced_only"
	// ReasonNoCondition is a preset rule without conditions, ReasonEmptyCondition a condition without values.
	ReasonNoCondition    = "no_condition"
	ReasonEmptyCondition = "empty_condition"
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
