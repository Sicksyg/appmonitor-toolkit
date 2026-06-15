package assets

import "embed"

// Android-related embedded assets
//
//go:embed android_permissions.json
var AndroidPermissions embed.FS

// SDK information embedded assets
//
//go:embed all_SDK.json
var AllSDK embed.FS

// iOS-related embedded assets
//
//go:embed ios_permissions.json
var IosPermissions embed.FS

//go:embed ios_signatures.json
var IosSignatures embed.FS

// ReadFile reads a file from the appropriate embedded filesystem based on filename.
// Used as a fallback when runtime file loading fails.
func ReadFile(name string) ([]byte, error) {
	switch name {
	case "android_permissions.json":
		return AndroidPermissions.ReadFile(name)
	case "all_SDK.json":
		return AllSDK.ReadFile(name)
	case "ios_permissions.json":
		return IosPermissions.ReadFile(name)
	case "ios_signatures.json":
		return IosSignatures.ReadFile(name)
	default:
		// Fallback: try to read from any FS (will fail with clear error if not found)
		return AndroidPermissions.ReadFile(name)
	}
}
