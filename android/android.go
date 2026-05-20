package android

import (
	"AppMonitor/models"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
)

type Manager struct {
	logger func(message, function string)
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
	err = os.WriteFile("all_sdks.json", body, 0644)
	if err != nil {
		m.logger(fmt.Sprintf("Error writing SDK data to file: %v", err), "Manager.GetSDKIdentifiersFromExodus")
		return
	}
	m.logger("SDK data saved to all_sdks.json", "Manager.GetSDKIdentifiersFromExodus")
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

	err = os.WriteFile(filename, formattedBody, 0644)
	if err != nil {
		m.logger(fmt.Sprintf("Error writing SDK data to file: %v", err), "Manager.GetSDKsFromExodus")
		return
	}
	m.logger("SDK data saved to all_sdks.json", "Manager.GetSDKsFromExodus")
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

// parse exodus data

func (m *Manager) RunCompleteAnalysis(bundleID string, authToken string) (map[string]models.AndroidPermissionDetail, map[string][]string, error) {
	// This function runs the complete analysis for the given bundleID and returns the results to the frontend
	m.logger(fmt.Sprintf("Running complete analysis for app: %s", bundleID), "Manager.RunCompleteAnalysis")
	// Analysis logic would go here

	// get exodus data for the app
	m.GetSDKsFromExodus(authToken)

	// Two steps:
	// 1. Parse the app data to extract permissions and SDKs
	// 2. Enrich the SDKs and permissions with additional information from the all_sdks.json file

	return nil, nil, nil
}
