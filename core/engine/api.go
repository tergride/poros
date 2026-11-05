package engine

// The types below are the contract with the interface: tygo turns them into src/platform/core-types.ts.

// AppInfo describes the running core.
type AppInfo struct {
	SingBox string `json:"singBox"`
	Xray    string `json:"xray"`
	DataDir string `json:"dataDir"`
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
	// Preset is the ID of the routing preset in use.
	Preset   string `json:"preset"`
	CheckIP  bool   `json:"checkIp"`
	AllowLAN bool   `json:"allowLan"`
	// SubscriptionAutoUpdate updates the subscriptions on a schedule, every SubscriptionUpdateHours hours; 0 hours
	// is what each provider advises, or 6 hours.
	SubscriptionAutoUpdate  bool `json:"subscriptionAutoUpdate"`
	SubscriptionUpdateHours int  `json:"subscriptionUpdateHours"`
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
}

// Preset is a routing preset: which traffic goes through the server.
type Preset struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Builtin bool   `json:"builtin"`
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
)

// Events the core sends without a request.
const (
	EventSettingsChanged      = "settings.changed"
	EventSubscriptionsChanged = "subscriptions.changed"
	EventServersChanged       = "servers.changed"
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
)
