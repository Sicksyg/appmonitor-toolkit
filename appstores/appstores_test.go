package appstores

import (
	"encoding/json"
	"testing"
)

func TestStoreResultDecodesMinimumOSVersion(t *testing.T) {
	var response StoreSearchResponse
	if err := json.Unmarshal([]byte(`{"results":[{"bundleId":"com.example.app","minimumOsVersion":"17.0"}]}`), &response); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if len(response.Results) != 1 || response.Results[0].MinimumOSVersion != "17.0" {
		t.Fatalf("MinimumOSVersion = %+v, want 17.0", response.Results)
	}
}
