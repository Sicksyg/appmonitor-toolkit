package models

import (
	"AppMonitor/helpers"
	"encoding/json"
	"fmt"
)

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

// InstalledApp represents an app installed on a device
// AppInfo struct to hold app information and installation details for the analysis
type AppInfo struct {
	Name               string                             `json:"name"`
	BundleID           string                             `json:"bundleId"`
	InstallPath        string                             `json:"installPath,omitempty"`
	UDID               string                             `json:"udid,omitempty"`
	ArtworkUrl         string                             `json:"artworkUrl,omitempty"`
	SellerName         string                             `json:"sellerName,omitempty"`
	ArtistViewUrl      string                             `json:"artistViewUrl,omitempty"`
	Description        string                             `json:"description,omitempty"`
	AppStoreURL        string                             `json:"appStoreUrl,omitempty"`
	AppStoreIconPath   string                             `json:"appStoreIconPath,omitempty"`
	InstalledApps      []helpers.InstalledApp             `json:"installedApps,omitempty"`
	ResultsPath        string                             `json:"resultsPath,omitempty"`
	Version            string                             `json:"version,omitempty"`
	AnalysisDate       string                             `json:"analysisDate"`
	SDKs               map[string][]string                `json:"sdks,omitempty"`
	IosPermissions     map[string]IosPermissionDetail     `json:"iosPermissions,omitempty"`
	BundleInfo         map[string]any                     `json:"bundleInfo,omitempty"`
	AndroidPermissions map[string]AndroidPermissionDetail `json:"androidPermissions,omitempty"`
}

// AnalysisDatabase keeps a history of dated analysis records for each bundle ID.
type AnalysisDatabase map[string][]AppInfo

// DecodeAnalysisDatabase accepts both the current history format and the
// previous single-record format so existing database files remain usable.
func DecodeAnalysisDatabase(data []byte) (AnalysisDatabase, error) {
	if len(data) == 0 {
		return make(AnalysisDatabase), nil
	}

	var rawEntries map[string]json.RawMessage
	if err := json.Unmarshal(data, &rawEntries); err != nil {
		return nil, fmt.Errorf("decode database: %w", err)
	}

	database := make(AnalysisDatabase, len(rawEntries))
	for bundleID, rawEntry := range rawEntries {
		var history []AppInfo
		if err := json.Unmarshal(rawEntry, &history); err == nil {
			database[bundleID] = history
			continue
		}

		var record AppInfo
		if err := json.Unmarshal(rawEntry, &record); err != nil {
			return nil, fmt.Errorf("decode record for %s: %w", bundleID, err)
		}
		database[bundleID] = []AppInfo{record}
	}

	return database, nil
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
