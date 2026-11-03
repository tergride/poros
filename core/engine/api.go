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
}

// Subscription is a link from a provider; its servers arrive when it is updated.
type Subscription struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	URL  string `json:"url"`
	// AddedAt and UpdatedAt are Unix milliseconds; UpdatedAt is 0 before the first update.
	AddedAt   int64 `json:"addedAt"`
	UpdatedAt int64 `json:"updatedAt"`
}

// Preset is a routing preset: which traffic goes through the server.
type Preset struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Builtin bool   `json:"builtin"`
}

// AddSubscription asks to add a subscription. An empty name takes the host of the URL.
type AddSubscription struct {
	URL  string `json:"url"`
	Name string `json:"name"`
}

// ByID names one object.
type ByID struct {
	ID string `json:"id"`
}

// Methods the core answers.
const (
	MethodAppInfo             = "app.info"
	MethodSettingsGet         = "settings.get"
	MethodSettingsSet         = "settings.set"
	MethodSubscriptionsList   = "subscriptions.list"
	MethodSubscriptionsAdd    = "subscriptions.add"
	MethodSubscriptionsRemove = "subscriptions.remove"
	MethodPresetsList         = "presets.list"
)

// Events the core sends without a request.
const (
	EventSettingsChanged      = "settings.changed"
	EventSubscriptionsChanged = "subscriptions.changed"
)

// Reasons in the data of a failed request, for the interface to translate.
const (
	ReasonInvalidURL    = "invalid_url"
	ReasonDuplicate     = "duplicate"
	ReasonNotFound      = "not_found"
	ReasonUnknownPreset = "unknown_preset"
	ReasonInvalidValue  = "invalid_value"
)
