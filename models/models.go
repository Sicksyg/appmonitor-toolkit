package models

// IosPermissionDetail struct to hold enriched permission information
// Used by both analysis and report packages to maintain clean separation
type IosPermissionDetail struct {
	CommonName           string `json:"commonName"`
	AppleDescription     string `json:"appleDescription"`
	DeveloperDescription string `json:"developerDescription"`
	Category             string `json:"category,omitempty"`
	PlistKey             string `json:"plistKey,omitempty"`
}

// AndroidPermissionDetail struct to hold enriched permission information for Android apps
type AndroidPermissionDetail struct {
	pkey                string `json:"pkey"`
	CommonName          string `json:"commonName"`
	DescriptionSimple   string `json:"descriptionSimple"`
	DescriptionDetailed string `json:"descriptionDetailed"`
	ProtectionLevel     string `json:"protectionLevel,omitempty"`
	Link                string `json:"link,omitempty"`
}

// Settings structs for app configuration persisted to disk.
type Settings struct {
	Auth          AuthSettings    `json:"auth"`
	Options       OptionsSettings `json:"options"`
	Report        ReportSettings  `json:"report"`
	ExodusAPIKey  ExodusAPIKey    `json:"exodusApiKey"`
	GoogleCookies GoogleCookies   `json:"googleCookies"`
}

type AuthSettings struct {
	AppleEmail     string `json:"AppleEmail"`
	ApplePassword  string `json:"ApplePassword"`
	GoogleEmail    string `json:"GoogleEmail"`
	GooglePassword string `json:"GooglePassword"`
}

type OptionsSettings struct {
	DownloadFromAppStore bool `json:"DownloadFromAppStore"`
	InstallOnDevice      bool `json:"InstallOnDevice"`
}

type ReportSettings struct {
	SavePath string `json:"SavePath"`
}

type ExodusAPIKey struct {
	Key string `json:"key"`
}

type GoogleCookies struct {
	Cookies string `json:"cookies"`
}
