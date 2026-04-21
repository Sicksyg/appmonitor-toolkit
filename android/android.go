package android

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

type SearchDetails struct {
	Name     string
	ImageURL string
	BundleID string
}

// StoreSearchResponse represents the structure of the search response from the Google Play Store
type StoreSearchResponse struct {
	ResultCount int           `json:"resultCount"`
	Results     []StoreResult `json:"results"`
}

// StoreResult represents the details of an individual app result from the Google Play Store search
type StoreResult struct {
	TrackName  string `json:"trackName"`
	BundleID   string `json:"bundleId"`
	SellerName string `json:"sellerName"`
	ImageURL   string `json:"artworkUrl60"`
}

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

func (m *Manager) GooglePlayDetails(bundleID string) StoreResult {
	// Fetch details of the app with the given bundle ID from the Google Play Store
	// Example URL: https://play.google.com/store/apps/details?id=dk.sundhed.minsundhed&hl=da

	url := "https://play.google.com/store/apps/details?id=" + bundleID + "&hl=da"
	resp, err := http.Get(url)
	if err != nil {
		m.logger(fmt.Sprintf("Error making request: %v", err), "Manager.GooglePlayDetails")
		return StoreResult{}
	}
	defer resp.Body.Close()

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		m.logger(fmt.Sprintf("Error parsing HTML: %v", err), "Manager.GooglePlayDetails")
		return StoreResult{}
	}

	var details StoreResult

	details.TrackName = doc.Find("h1[itemprop='name']").First().Text()
	details.BundleID = bundleID
	details.SellerName = doc.Find("a[href*='/store/apps/developer?id=']").First().Text()
	details.ImageURL = doc.Find("img[itemprop='image']").First().AttrOr("src", "")

	return details
}

func (m *Manager) GooglePlaySearch(searchTerm string) string {
	// Search the Google Play Store for the given search term
	// Example URL: https://play.google.com/store/search?q=sundhed&c=apps&hl=da

	url := "https://play.google.com/store/search?q=" + searchTerm + "&c=apps&hl=da"

	resp, err := http.Get(url)
	if err != nil {
		m.logger(fmt.Sprintf("Error making request: %v", err), "Manager.GooglePlaySearch")
		return "{}"
	}
	defer resp.Body.Close()

	// Parse HTML and extract app information
	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		m.logger(fmt.Sprintf("Error parsing HTML: %v", err), "Manager.GooglePlaySearch")
		return "{}"
	}

	var searchResults StoreSearchResponse

	// Extract app links and names (limited by the limit parameter) by iterating over the search results
	doc.Find("a[href*='/store/apps/details']").Each(func(i int, s *goquery.Selection) {
		if len(searchResults.Results) >= 5 { // Limit to 5 results
			return
		}
		href, exists := s.Attr("href")
		// Extract bundle ID from the href and fetch details for each app
		if exists {
			bundleID := strings.Split(href, "id=")[1]
			details := m.GooglePlayDetails(bundleID)
			searchResults.Results = append(searchResults.Results, details)
		}
	})

	searchResults.ResultCount = len(searchResults.Results)

	jsonBytes, err := json.Marshal(searchResults)
	if err != nil {
		m.logger(fmt.Sprintf("Error encoding JSON: %v", err), "Manager.GooglePlaySearch")
		return "{}"
	}

	m.logger(fmt.Sprintf("Google Play search results for '%s': %s", searchTerm, string(jsonBytes)), "Manager.GooglePlaySearch")

	return string(jsonBytes)
}

func (m *Manager) DownloadAndroidApp(bundleID string) {
	// Download the apk using apkeep
	m.logger(fmt.Sprintf("Downloading Android app with bundle ID: %s", bundleID), "Manager.DownloadAndroidApp")

	/* apkeep -a md.point.news -d google-play -e 'EMAIL_HERE' -t 'TOKEN_HERE' . */

	// ONLY FOR TESTING:
	email := "<INSERT_EMAIL_HERE>"
	token := "<INSERT_TOKEN_HERE>"
	appkeepCmd := exec.Command("apkeep", "-a", bundleID, "-d", "google-play", "-e", email, "-t", token, "./output")
	output, err := appkeepCmd.CombinedOutput()
	if err != nil {
		m.logger(fmt.Sprintf("Error running apkeep: %v", err), "Manager.DownloadAndroidApp")
		return
	}
	m.logger(fmt.Sprintf("apkeep output: %s", string(output)), "Manager.DownloadAndroidApp")
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

func (m *Manager) GetAppDataFromExodus(bundleID string, authToken string) {
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
		return
	}

	for key, value := range headers {
		req.Header.Set(key, value)

	}
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		m.logger(fmt.Sprintf("Error making request: %v", err), "Manager.GetAppDataFromExodus")
		return
	}
	defer resp.Body.Close()

	// parse response and print app data
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		m.logger(fmt.Sprintf("Error reading response body: %v", err), "Manager.GetAppDataFromExodus")
		return
	}
	m.logger(fmt.Sprintf("App data for %s was successfully fetched from Exodus API", bundleID), "Manager.GetAppDataFromExodus")

	// save to file
	var filename string = fmt.Sprintf("./output/%s_data.json", bundleID)

	err = os.WriteFile(filename, body, 0644)
	if err != nil {
		m.logger(fmt.Sprintf("Error writing app data to file: %v", err), "Manager.GetAppDataFromExodus")
		return
	}
	m.logger(fmt.Sprintf("App data for %s saved to %s", bundleID, filename), "Manager.GetAppDataFromExodus")
}

func (m *Manager) AnalyzeAndroidApp(filePath string) {
	// Analyze the downloaded apk file using Exodus / Dexdump
	m.logger(fmt.Sprintf("Analyzing Android app at: %s", filePath), "Manager.AnalyzeAndroidApp")
	// Analysis logic would go here
}
