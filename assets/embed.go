package assets

import (
	"embed"
	"fmt"
	"io/fs"
	"path/filepath"
)

// This file contains embedded assets used by the app. The assets are embedded using the Go 1.16 embed package.
// The assets include JSON files and macOS binaries that are required for the app's functionality. If not, the files are not included in the binary, and the app will not work correctly.
// Assets are avaliable in the DataFS and BinFS variables. To acces fx the ios_signatures.json file, use the ReadFile function. To access the macOS binaries, use the ReadBundledBinary function.
// FX:
// data, err := assets.ReadFile("ios_signatures.json")
// binary, err := assets.ReadBundledBinary("arm64", "ipatool")
//
//go:embed android_permissions.json all_SDK.json ios_permissions.json ios_signatures.json
var DataFS embed.FS

// BinFS contains the embedded macOS binaries.
// It is kept separate so callers can list the files inside the directory.
//
//go:embed bin/darwin-arm64/*
var BinFS embed.FS

//go:embed lib/*
var LibFS embed.FS

//go:embed frida
var FridaFS embed.FS

// ReadFile returns the contents of one embedded JSON asset.
func ReadFile(name string) ([]byte, error) {
	return DataFS.ReadFile(name)
}

// ReadBundledBinary reads one embedded executable from bin/darwin-<arch>/.
func ReadBundledBinary(goarch, filename string) ([]byte, error) {
	p := filepath.Join("bin", "darwin-"+goarch, filename)
	b, err := BinFS.ReadFile(p)
	if err != nil {
		return nil, fmt.Errorf("embedded binary not found %s: %w", p, err)
	}
	return b, nil
}

func ReadBundledLib(goarch, filename string) ([]byte, error) {
	p := filepath.Join("lib", "darwin-"+goarch, filename)
	b, err := LibFS.ReadFile(p)
	if err != nil {
		return nil, fmt.Errorf("embedded lib not found %s: %w", p, err)
	}
	return b, nil
}

// ListBundledBinaries lists all embedded files in bin/darwin-<arch>/.
func ListBundledBinaries(goarch string) ([]fs.DirEntry, error) {
	p := filepath.Join("bin", "darwin-"+goarch)
	return fs.ReadDir(BinFS, p)
}

// ListBundledLibs lists all embedded files in lib/darwin-<arch>/.
func ListBundledLibs(goarch string) ([]fs.DirEntry, error) {
	p := filepath.Join("lib", "darwin-"+goarch)
	return fs.ReadDir(LibFS, p)
}
