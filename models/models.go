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
	PKey                string `json:"pkey"`
	CommonName          string `json:"commonName"`
	DescriptionSimple   string `json:"descriptionSimple"`
	DescriptionDetailed string `json:"descriptionDetailed"`
	ProtectionLevel     string `json:"protectionLevel,omitempty"`
	Link                string `json:"link,omitempty"`
}

// AndroidSdkDetail struct to hold enriched information about Android SDKs, used for both analysis and reporting
type AndroidSdkDetail struct {
	ID            int      `json:"id"`
	Name          string   `json:"name"`
	Website       string   `json:"website"`
	CodeSignature string   `json:"code_signature"`
	Documentation []string `json:"documentation"`
}

// ExodusTrackerFile struct to represent the structure of the Exodus tracker data file. This allows for easy loading and access.
type ExodusTrackerFile struct {
	Trackers map[string]AndroidSdkDetail `json:"trackers"`
}

// Settings structs for app configuration persisted to disk.
type Settings struct {
	Auth          AuthSettings    `json:"auth"`
	Options       OptionsSettings `json:"options"`
	Output        OutputSetting   `json:"Output"`
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

type OutputSetting struct {
	OutputPath     string `json:"OutputPath"`
	ReportSavePath string `json:"ReportSavePath"`
}

type ExodusAPIKey struct {
	Key string `json:"key"`
}

type GoogleCookies struct {
	Cookies string `json:"cookies"`
}
