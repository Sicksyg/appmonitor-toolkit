# analysis package

This directory contains AppMonitor analysis logic and tests.

## Test files
- analysis_test.go: unit tests that do not require a live device
- analysis_live_test.go: optional live Frida integration tests

## Running tests
Run unit tests only:

```bash
go test ./analysis
```

Run live tests (requires connected Frida-capable device):

```bash
export APPMONITOR_FRIDA_UDID=<your-device-udid>
export APPMONITOR_BUNDLE_ID=dk.dr.tv
go test -tags livefrida ./analysis -run LiveFrida -v -count=1
```

Use short mode to skip long-running live tests:

```bash
go test -tags livefrida ./analysis -short
```

## Run analysis without GUI
Use the CLI runner to execute Frida analysis directly from terminal (no Wails dev reloads):

```bash
go run ./cmd/frida-analysis -udid 32a5b4d0c84ba01c4f35d7eb3c31c283fadedcec -bundle dk.dr.tv
```

Quick setup-only check (spawn/attach + cleanup):

```bash
go run ./cmd/frida-analysis -udid 32a5b4d0c84ba01c4f35d7eb3c31c283fadedcec -bundle dk.dr.tv -setup-only
```

Write JSON output to a file:

```bash
go run ./cmd/frida-analysis -udid 32a5b4d0c84ba01c4f35d7eb3c31c283fadedcec -bundle dk.dr.tv -json-out tmp/drtv_analysis.json
```

## Live test configuration
Environment variables used by live tests:
- APPMONITOR_FRIDA_UDID (required): target device UDID
- APPMONITOR_BUNDLE_ID (optional): target bundle ID, defaults to dk.dr.tv

Default live app context used in tests:
- trackName: DRTV
- trackId: 877604999
- bundleId: dk.dr.tv

## Prerequisites for live tests
- Frida host tooling installed and working
- Frida server available on the target device
- The target app installed on the device
