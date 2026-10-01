# iOS IPA compatibility preparation

AppMonitor handles iOS IPA compatibility preparation in Go through
[`ios/ipa.go`](../ios/ipa.go). The application no longer runs the former
Python-dependent shell installer.

## Install flow

For apps whose selected App Store lookup minimum is iOS 16.x through 18.x,
AppMonitor downloads the IPA and uses `ditto` to extract it. It lowers the app
and retained extension `MinimumOSVersion` values to 16.0 when those values are
higher or missing. Thin arm64 Mach-O minimum-OS load commands are lowered too;
changed executables are re-signed with `ldid`, preserving extractable
entitlements.

The first install attempt keeps all extensions. If it fails and there are
extensions whose extension-point identifiers are not in the compatibility
allowlist, AppMonitor creates a pruned IPA and retries once. The original
downloaded IPA is not modified. Apps requiring iOS 19 or later are rejected
before installation.

## Runtime tools

- `ditto` is provided by macOS. Set `APPMONITOR_DITTO_PATH` to use another
  executable.
- `ldid` is only needed when a Mach-O minimum-OS load command requires a
  change. AppMonitor uses `APPMONITOR_LDID_PATH` when configured, otherwise
  checks its extracted bundled-tools path and then searches `PATH`. To bundle
  it for Apple Silicon, add `ldid` to `assets/bin/darwin-arm64/`; required
  dylibs can be placed in `assets/lib/darwin-arm64/` and are made available
  to `ldid` from the extracted library directory.
- IPA property lists are read and written using the Go
  [`howett.net/plist`](https://pkg.go.dev/howett.net/plist) package. Python,
  `unzip`, and `zip` are not required.

Set `APPMONITOR_COMPATIBLE_EXTENSION_POINTS` to a whitespace-separated list
to replace the built-in extension-point allowlist. Unknown extension points
are pruned when a retry is needed.

## Limitations

Lowering a declared minimum version only bypasses an installation preflight
check; it does not add APIs or OS features missing from the device. Editing a
Mach-O invalidates its original code signature, so `ldid` creates a new
signature rather than preserving Apple's App Store signature. The device's
jailbreak/signing configuration determines whether the resulting app can be
installed and run.

Pruning removes whole `.appex` bundles, including their features and any
SDKs contained only in those extensions. A successful installation does not
guarantee launch or runtime analysis on an older iOS version.
