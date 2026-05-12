package itunes

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

type Manager struct {
	logger func(message, function string)
}

func NewManager(logger func(message, function string)) *Manager {
	return &Manager{
		logger: logger,
	}
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

	var result struct {
		ResultCount int `json:"resultCount"`
		Results     []struct {
			TrackName     string `json:"trackName"`
			BundleID      string `json:"bundleId"`
			TrackID       int64  `json:"trackId"`
			SellerName    string `json:"sellerName"`
			ArtistViewUrl string `json:"artistViewUrl"`
			ArtworkUrl60  string `json:"artworkUrl60"`
			ArtworkUrl100 string `json:"artworkUrl100"`
		} `json:"results"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		a.logger("Error parsing iTunes response: "+err.Error(), "App.fetchItunesResults")
		return ""
	}

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
