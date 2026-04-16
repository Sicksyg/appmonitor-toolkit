package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"AppMonitor/analysis"
	"AppMonitor/models"
)

type runResult struct {
	BundleID    string                             `json:"bundleId"`
	UDID        string                             `json:"udid"`
	Timestamp   string                             `json:"timestamp"`
	Permissions map[string]models.PermissionDetail `json:"permissions"`
	SDKs        map[string][]string                `json:"sdks"`
}

func main() {
	udid := flag.String("udid", "", "device UDID (required)")
	bundleID := flag.String("bundle", "dk.dr.tv", "target app bundle ID")
	setupOnly := flag.Bool("setup-only", false, "only validate Frida setup + cleanup")
	jsonOut := flag.String("json-out", "", "optional output path for JSON results")
	flag.Parse()

	if *udid == "" {
		log.Fatal("missing required -udid flag")
	}

	if err := os.MkdirAll("tmp", 0o755); err != nil {
		log.Fatalf("failed creating tmp directory: %v", err)
	}

	logger := func(message, function string) {
		log.Printf("%s: %s", function, message)
	}

	mgr := analysis.NewManager(logger)
	defer func() {
		if err := mgr.Cleanup(); err != nil {
			log.Printf("cleanup warning: %v", err)
		}
	}()

	if *setupOnly {
		if err := mgr.FridaSetup(*udid, *bundleID); err != nil {
			log.Fatalf("frida setup failed: %v", err)
		}
		fmt.Printf("Frida setup succeeded for bundleId=%s udid=%s\n", *bundleID, *udid)
		return
	}

	permissions, sdks, err := mgr.RunCompleteAnalysis(*udid, *bundleID)
	if err != nil {
		log.Fatalf("analysis failed: %v", err)
	}

	result := runResult{
		BundleID:    *bundleID,
		UDID:        *udid,
		Timestamp:   time.Now().UTC().Format(time.RFC3339),
		Permissions: permissions,
		SDKs:        sdks,
	}

	out, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		log.Fatalf("failed to encode result JSON: %v", err)
	}

	if *jsonOut != "" {
		if err := os.MkdirAll(filepath.Dir(*jsonOut), 0o755); err != nil {
			log.Fatalf("failed to create output directory: %v", err)
		}
		if err := os.WriteFile(*jsonOut, out, 0o644); err != nil {
			log.Fatalf("failed writing output JSON: %v", err)
		}
		fmt.Printf("Analysis complete. Results written to %s\n", *jsonOut)
		return
	}

	fmt.Println(string(out))
}
