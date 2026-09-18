package appstores

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/PuerkitoBio/goquery"
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

// StoreSearchResponse represents the structure of the search response from the Google Play Store
type StoreSearchResponse struct {
	ResultCount int           `json:"resultCount"`
	Results     []StoreResult `json:"results"`
}

// StoreResult represents the details of an individual app result from the appstores
type StoreResult struct {
	TrackID       int64  `json:"trackId"`
	TrackName     string `json:"trackName"`
	BundleID      string `json:"bundleId"`
	SellerName    string `json:"sellerName"`
	ArtistViewURL string `json:"artistViewUrl"`
	ArtworkURL    string `json:"artworkUrl512"`
	Description   string `json:"description"`
	TrackViewUrl  string `json:"trackViewUrl"`
}

func (a *Manager) fetchItunesResults(url string) string {
	resp, err := http.Get(url)
	if err != nil {
		a.logger("Error searching iTunes: "+err.Error(), "App.fetchItunesResults")
		return ""
	}

	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		a.logger("Error reading iTunes response: "+err.Error(), "App.fetchItunesResults")
		return ""
	}

	var result StoreSearchResponse

	if err := json.Unmarshal(body, &result); err != nil {
		a.logger("Error parsing iTunes response: "+err.Error(), "App.fetchItunesResults")
		return ""
	}

	//fmt.Printf("iTunes search results: %+v\n", result)

	jsonBytes, _ := json.Marshal(result)
	return string(jsonBytes)
}

func (a *Manager) ItunesSearchBundle(bundleID string) string {
	// This function searches iTunes for the given bundleID and returns the result
	// Build iTunes Search API URL with the bundleID
	// Example URL with instagram: https://itunes.apple.com/lookup?bundleId=com.burbn.instagram&country=dk
	url := fmt.Sprintf("https://itunes.apple.com/lookup?bundleId=%s&country=dk", bundleID) // Should maybe change country code based on device info

	return a.fetchItunesResults(url)
}

func (a *Manager) ItunesSearchWild(term string) string {
	// Documentation: https://developer.apple.com/library/archive/documentation/AudioVideo/Conceptual/iTuneSearchAPI/SearchExamples.html#//apple_ref/doc/uid/TP40017632-CH6-SW1
	// This function searches iTunes for the given term and returns the result
	a.logger("Searching iTunes for: "+term, "App.ItunesSearch")

	// slice spaces with +
	term = strings.ReplaceAll(term, " ", "+")
	// Encode special chars (ÆØÅ, etc.) but keep our + separators
	term = url.QueryEscape(term)
	term = strings.ReplaceAll(term, "%2B", "+")

	// Build iTunes Search API URL with the term
	url := fmt.Sprintf("https://itunes.apple.com/search?term=%s&country=dk&entity=software&limit=5", term) // Should maybe change country code based on device info

	return a.fetchItunesResults(url)
}

func (m *Manager) GooglePlayDetails(bundleID string) StoreResult {
	// Fetch details of the app with the given bundle ID from the Google Play Store
	// Example URL: https://play.google.com/store/apps/details?id=dk.sundhed.minsundhed&hl=da

	playURL := "https://play.google.com/store/apps/details?id=" + url.QueryEscape(bundleID) + "&hl=da"
	resp, err := http.Get(playURL)
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

	details.TrackName = strings.TrimSpace(doc.Find("h1[itemprop='name']").First().Text())
	if details.TrackName == "" {
		details.TrackName = strings.TrimSpace(doc.Find("meta[property='og:title']").First().AttrOr("content", ""))
	}
	details.BundleID = bundleID
	details.SellerName = strings.TrimSpace(doc.Find("a[href*='/store/apps/developer?id=']").First().Text())
	details.ArtistViewURL = playURL
	details.ArtworkURL = strings.TrimSpace(doc.Find("img[itemprop='image']").First().AttrOr("src", ""))
	details.Description = strings.TrimSpace(doc.Find("meta[property='og:description']").First().AttrOr("content", ""))
	if details.Description == "" {
		details.Description = strings.TrimSpace(doc.Find("meta[name='description']").First().AttrOr("content", ""))
	}
	details.TrackViewUrl = playURL

	return details
}

func (m *Manager) GooglePlaySearch(searchTerm string) string {
	// Search the Google Play Store for the given search term
	// Example URL: https://play.google.com/store/search?q=sundhed&c=apps&hl=da

	searchURL := "https://play.google.com/store/search?q=" + searchTerm + "&c=apps&hl=da"

	resp, err := http.Get(searchURL)
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
		if !exists {
			return
		}

		if strings.HasPrefix(href, "/") {
			href = "https://play.google.com" + href
		}

		parsedURL, err := url.Parse(href)
		if err != nil {
			return
		}

		bundleID := parsedURL.Query().Get("id")
		if bundleID == "" {
			return
		}

		details := m.GooglePlayDetails(bundleID)
		searchResults.Results = append(searchResults.Results, details)
	})

	searchResults.ResultCount = len(searchResults.Results)

	jsonBytes, err := json.Marshal(searchResults)
	if err != nil {
		m.logger(fmt.Sprintf("Error encoding JSON: %v", err), "Manager.GooglePlaySearch")
		return "{}"
	}

	//m.logger(fmt.Sprintf("Google Play search results for '%s': %s", searchTerm, string(jsonBytes)), "Manager.GooglePlaySearch")

	return string(jsonBytes)
}

// func (m *Manager) DownloadAndroidApp(bundleID string, email string, token string) {
// 	// Download the apk using apkeep
// 	m.logger(fmt.Sprintf("Downloading Android app with bundle ID: %s", bundleID), "Manager.DownloadAndroidApp")

// 	/* apkeep -a md.point.news -d google-play -e 'EMAIL_HERE' -t 'TOKEN_HERE' . */

// 	appkeepCmd := exec.Command("apkeep", "-a", bundleID, "-d", "google-play", "-e", email, "-t", token, "./output")
// 	output, err := appkeepCmd.CombinedOutput()
// 	if err != nil {
// 		m.logger(fmt.Sprintf("Error running apkeep: %v", err), "Manager.DownloadAndroidApp")
// 		return
// 	}
// 	m.logger(fmt.Sprintf("apkeep output: %s", string(output)), "Manager.DownloadAndroidApp")
// }

// func main() {
// 	// Example usage
// 	manager := NewManager(func(message, function string) {
// 		fmt.Printf("[%s] %s\n", function, message)
// 	})

// 	// Example calls
// 	manager.ItunesSearchWild("MinLæge")
// 	fmt.Println("\n-----\n")
// 	manager.GooglePlaySearch("Min Læge")

// }
