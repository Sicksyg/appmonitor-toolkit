package android

import (
	assetfiles "AppMonitor/assets"
	"AppMonitor/models"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
)

type Manager struct {
	logger func(message, function string)
}

type Permission struct {
	Name string
}

// NewManager creates a new analysis Manager
func NewManager(logger func(message, function string)) *Manager {
	return &Manager{
		logger: logger,
	}
}

func (m *Manager) GrapCookies() {
	m.logger("GrapCookies function called", "Manager.GrapCookies")
	// Android-specific cookie grabbing logic would go here
	url := "https://accounts.google.com/v3/signin/identifier?flowName=EmbeddedSetupAndroid&continue=https://accounts.google.com/o/android/auth?lang%3Den%26cc%3DUS%26langCountry%3Den_US%26xoauth_display_name%3DAndroid%2BDevice%26tmpl%3Dnew_account%26source%3Dandroid%26return_user_id%3Dtrue&dsh=S1226023185:1769769569605796"
	resp, err := http.Get(url)
	if err != nil {
		m.logger("Error making request: "+err.Error(), "Manager.GrapCookies")
		return
	}
	defer resp.Body.Close()

	for _, cookie := range resp.Cookies() {
		m.logger(fmt.Sprintf("Cookie: %s = %s", cookie.Name, cookie.Value), "Manager.GrapCookies")
	}
}

func (m *Manager) GetSDKIdentifiersFromExodus(authToken string) {
	// Fetch SDK data from the Exodus API
	m.logger("Fetching SDK data from Exodus API", "Manager.GetSDKIdentifiersFromExodus")
	// API call logic would go here

	url := "https://reports.exodus-privacy.eu.org/api/trackers"
	headers := map[string]string{
		"Authorization": "Token " + authToken, // <-- INSERT TOKEN HERE --
	}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		m.logger(fmt.Sprintf("Error making request: %v", err), "Manager.GetSDKIdentifiersFromExodus")
		return
	}
	for key, value := range headers {
		req.Header.Set(key, value)
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		m.logger(fmt.Sprintf("Error making request: %v", err), "Manager.GetSDKIdentifiersFromExodus")
		return
	}
	defer resp.Body.Close()

	// parse response and save SDK data to a json file
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		m.logger(fmt.Sprintf("Error reading response body: %v", err), "Manager.GetSDKIdentifiersFromExodus")
		return
	}
	err = os.WriteFile("all_SDK.json", body, 0644)
	if err != nil {
		m.logger(fmt.Sprintf("Error writing SDK data to file: %v", err), "Manager.GetSDKIdentifiersFromExodus")
		return
	}
	m.logger("SDK data saved to all_SDK.json", "Manager.GetSDKIdentifiersFromExodus")
}

func (m *Manager) GetSDKsFromExodus(authToken string) {
	// Fetch SDK data from the Exodus API
	m.logger("Fetching SDK data from Exodus API", "Manager.GetSDKsFromExodus")
	// API call logic would go here

	url := "https://reports.exodus-privacy.eu.org/api/trackers"
	headers := map[string]string{
		"Authorization": "Token " + authToken, // <-- INSERT TOKEN HERE --
	}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		m.logger(fmt.Sprintf("Error making request: %v", err), "Manager.GetSDKsFromExodus")
		return
	}
	for key, value := range headers {
		req.Header.Set(key, value)
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		m.logger(fmt.Sprintf("Error making request: %v", err), "Manager.GetSDKsFromExodus")
		return
	}
	defer resp.Body.Close()

	// parse response and save SDK data to a json file
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		m.logger(fmt.Sprintf("Error reading response body: %v", err), "Manager.GetSDKsFromExodus")
		return
	}

	var parsed interface{}
	err = json.Unmarshal(body, &parsed)
	if err != nil {
		m.logger(fmt.Sprintf("Error parsing response JSON: %v", err), "Manager.GetSDKsFromExodus")
		return
	}

	formattedBody, err := json.MarshalIndent(parsed, "", "    ")
	if err != nil {
		m.logger(fmt.Sprintf("Error formatting JSON: %v", err), "Manager.GetSDKsFromExodus")
		return
	}

	filename := "./assets/all_SDK.json"

	err = os.MkdirAll("./assets", 0755)
	if err != nil {
		m.logger(fmt.Sprintf("Error creating assets directory: %v", err), "Manager.GetSDKsFromExodus")
		return
	}

	err = os.WriteFile(filename, formattedBody, 0644)
	if err != nil {
		m.logger(fmt.Sprintf("Error writing SDK data to file: %v", err), "Manager.GetSDKsFromExodus")
		return
	}
	m.logger("SDK data saved to all_SDK.json", "Manager.GetSDKsFromExodus")
}

// GetAppDataFromExodus fetches app data from the Exodus API for the given bundleID and saves it to a json file
func (m *Manager) GetAppDataFromExodus(bundleID string, authToken string, saveToFile bool) ([]byte, error) {
	// Fetch app data from the Exodus API for the given app name
	m.logger(fmt.Sprintf("Fetching data for app: %s from Exodus API", bundleID), "Manager.GetAppDataFromExodus")
	// API call logic would go here

	url := "https://reports.exodus-privacy.eu.org/api/search/" + url.PathEscape(bundleID) + "/details"
	headers := map[string]string{
		"Authorization": "Token " + authToken, // <-- INSERT TOKEN HERE --
	}

	req, err := http.NewRequest("GET", url, nil)

	if err != nil {
		m.logger(fmt.Sprintf("Error making request: %v", err), "Manager.GetAppDataFromExodus")
		return nil, err
	}

	for key, value := range headers {
		req.Header.Set(key, value)

	}
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		m.logger(fmt.Sprintf("Error making request: %v", err), "Manager.GetAppDataFromExodus")
		return nil, err
	}
	defer resp.Body.Close()

	// parse response and print app data
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		m.logger(fmt.Sprintf("Error reading response body: %v", err), "Manager.GetAppDataFromExodus")
		return nil, err
	}

	var items []json.RawMessage
	if err := json.Unmarshal(body, &items); err != nil {
		return nil, fmt.Errorf("error parsing JSON response: %v", err)
	}
	if len(items) == 0 {
		return nil, fmt.Errorf("no data found for app: %s", bundleID)
	}

	// Only take the last item in the response, which should be the most recent analysis for the app
	last := items[len(items)-1]
	formattedJson, err := json.MarshalIndent(last, "", "  ")
	if err != nil {
		return nil, err
	}

	m.logger(fmt.Sprintf("App data for %s was successfully fetched from Exodus API", bundleID), "Manager.GetAppDataFromExodus")

	if saveToFile {
		// save to file in tmp folder with the name {bundleID}_data.json
		filename := fmt.Sprintf("./tmp/%s_data.json", bundleID)

		err = os.WriteFile(filename, formattedJson, 0644)
		if err != nil {
			m.logger(fmt.Sprintf("Error writing app data to file: %v", err), "Manager.GetAppDataFromExodus")
			return nil, err
		}
		m.logger(fmt.Sprintf("App data for %s saved to %s", bundleID, filename), "Manager.GetAppDataFromExodus")
	}
	return formattedJson, err
}

func (m *Manager) EnrichPermissions(permissionList []Permission) (map[string]models.AndroidPermissionDetail, error) {
	// This function takes a list of permissions and enriches them with additional information from the android_permissions.json file
	m.logger("Enriching permissions with additional information", "Manager.EnrichPermissions")
	// Enrichment logic would go here

	// Load android_permissions.json file
	fileData, err := os.ReadFile("./assets/android_permissions.json")
	if err != nil {
		fileData, err = assetfiles.ReadFile("android_permissions.json")
	}
	if err != nil {
		m.logger(fmt.Sprintf("Error reading android_permissions.json file: %v", err), "Manager.EnrichPermissions")
		return nil, err
	}

	var permissionDetails struct {
		Permissions map[string]models.AndroidPermissionDetail `json:"permissions"`
	}
	err = json.Unmarshal(fileData, &permissionDetails)
	if err != nil {
		m.logger(fmt.Sprintf("Error parsing android_permissions.json file: %v", err), "Manager.EnrichPermissions")
		return nil, err
	}

	enrichedPermissions := make(map[string]models.AndroidPermissionDetail)

	for _, permission := range permissionList {
		found := false
		for _, detail := range permissionDetails.Permissions {
			if detail.PKey == permission.Name {
				enrichedPermissions[permission.Name] = detail
				found = true
				break
			}
		}
		if !found {
			m.logger(fmt.Sprintf("Permission %s not found in android_permissions.json", permission.Name), "Manager.EnrichPermissions")
		}
	}

	return enrichedPermissions, nil
}

func (m *Manager) EnrichSDKs(sdkIdList []int) (map[string]models.AndroidSdkDetail, error) {
	// This function takes a list of SDKs and enriches them with additional information from the all_SDK.json file
	m.logger("Enriching SDKs with additional information", "Manager.EnrichSDKs")
	// Enrichment logic would go here

	// Load all_SDK.json file
	fileData, err := os.ReadFile("./assets/all_SDK.json")
	if err != nil {
		fileData, err = assetfiles.ReadFile("all_SDK.json")
	}
	if err != nil {
		m.logger(fmt.Sprintf("Error reading all_SDK.json file: %v", err), "Manager.EnrichSDKs")
		return nil, err
	}

	var payload models.ExodusTrackerFile
	if err = json.Unmarshal(fileData, &payload); err != nil {
		m.logger(fmt.Sprintf("Error parsing all_SDK.json file: %v", err), "Manager.EnrichSDKs")
		return nil, err
	}

	// Loop through the list of SDK IDs and find the corresponding SDK details in the payload, then add them to the enrichedSDKs map as sdkname: SDKDetail
	enrichedSDKs := make(map[string]models.AndroidSdkDetail, len(sdkIdList))
	for _, sdkId := range sdkIdList {
		// Convert sdkId using strconv.Itoa(sdkId) for direct map lookup since the keys in the payload are strings
		sdkDetail, exists := payload.Trackers[strconv.Itoa(sdkId)]
		if exists {
			enrichedSDKs[sdkDetail.Name] = sdkDetail
		} else {
			m.logger(fmt.Sprintf("SDK with ID %d not found in all_SDK.json", sdkId), "Manager.EnrichSDKs")
		}
	}
	return enrichedSDKs, nil
}

func (m *Manager) RunCompleteAnalysis(bundleID string, authToken string) (map[string]models.AndroidPermissionDetail, map[string]models.AndroidSdkDetail, error) {
	// This function runs the complete analysis for the given bundleID and returns the results to the frontend
	m.logger(fmt.Sprintf("Running complete analysis for app: %s", bundleID), "Manager.RunCompleteAnalysis")
	// Analysis logic would go here

	// get exodus data for the app
	m.GetSDKsFromExodus(authToken)

	// 1. Fetch app data from Exodus API

	appdata, err := m.GetAppDataFromExodus(bundleID, authToken, false)
	if err != nil {
		m.logger(fmt.Sprintf("Error fetching app data: %v", err), "Manager.RunCompleteAnalysis")
		return nil, nil, err
	}

	var appDataParsed map[string]interface{}
	err = json.Unmarshal(appdata, &appDataParsed)
	if err != nil {
		m.logger(fmt.Sprintf("Error parsing app data JSON: %v", err), "Manager.RunCompleteAnalysis")
		return nil, nil, err
	}

	// 2 Enrich permissions with additional information from android_permissions.json. The permissions is in the form of a list of strings in appDataParsed["permissions"].
	permissionsList, ok := appDataParsed["permissions"].([]interface{})
	if !ok {
		m.logger("Error parsing permissions from app data", "Manager.RunCompleteAnalysis")
		return nil, nil, fmt.Errorf("error parsing permissions from app data")
	}

	simplePermissions := make([]Permission, 0)
	for _, p := range permissionsList {
		if permStr, ok := p.(string); ok {
			simplePermissions = append(simplePermissions, Permission{Name: permStr})
		}
	}

	enrichedPermissions, err := m.EnrichPermissions(simplePermissions)
	if err != nil {
		m.logger(fmt.Sprintf("Error enriching permissions: %v", err), "Manager.RunCompleteAnalysis")
		return nil, nil, err
	}

	// 3. Enrich SDKs with additional information from all_SDK.json. The SDKs is in the form of a list of strings in appDataParsed["sdks"].
	sdksList, ok := appDataParsed["trackers"].([]interface{})
	if !ok {
		m.logger("Error parsing SDKs from app data", "Manager.RunCompleteAnalysis")
		return nil, nil, fmt.Errorf("error parsing SDKs from app data")
	}

	// Loops through the list of SDKs and converts them to integers, then adds them to the sdkIdList
	sdkIdList := make([]int, 0) // Convert the list of SDKs from []interface{} to a map[int] for easier lookup
	for _, s := range sdksList {
		if sdkInt, ok := s.(float64); ok {
			sdkIdList = append(sdkIdList, int(sdkInt))
		}
	}

	// Calls the EnrichSDKs function to get the enriched SDK details for the list of SDK IDs
	enrichedSDKs, err := m.EnrichSDKs(sdkIdList)
	if err != nil {
		m.logger(fmt.Sprintf("Error enriching SDKs: %v", err), "Manager.RunCompleteAnalysis")
		return nil, nil, err
	}

	fmt.Printf("Enriched Permissions: %+v\n", enrichedPermissions)
	fmt.Printf("Enriched SDKs: %+v\n", enrichedSDKs)

	return enrichedPermissions, enrichedSDKs, nil
}
