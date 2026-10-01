package ios

import (
	"archive/zip"
	"context"
	"encoding/binary"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"howett.net/plist"
)

func TestPackedVersion(t *testing.T) {
	tests := []struct {
		version string
		want    uint32
		wantErr bool
	}{
		{version: "16.0", want: 16 << 16},
		{version: "17.2.1", want: 17<<16 | 2<<8 | 1},
		{version: "16.256", wantErr: true},
		{version: "16..1", wantErr: true},
		{version: "16.0.0.1", wantErr: true},
	}
	for _, test := range tests {
		got, err := packedVersion(test.version)
		if (err != nil) != test.wantErr {
			t.Errorf("packedVersion(%q) error = %v, wantErr %t", test.version, err, test.wantErr)
			continue
		}
		if err == nil && got != test.want {
			t.Errorf("packedVersion(%q) = %#x, want %#x", test.version, got, test.want)
		}
	}
}

func TestPatchMachOMinimumOS(t *testing.T) {
	data := make([]byte, 32+24)
	binary.LittleEndian.PutUint32(data[0:4], 0xfeedfacf)
	binary.LittleEndian.PutUint32(data[4:8], 0x0100000c)
	binary.LittleEndian.PutUint32(data[16:20], 1)
	binary.LittleEndian.PutUint32(data[32:36], 0x32)
	binary.LittleEndian.PutUint32(data[36:40], 24)
	binary.LittleEndian.PutUint32(data[40:44], 2)
	binary.LittleEndian.PutUint32(data[44:48], 18<<16)
	binary.LittleEndian.PutUint32(data[48:52], 18<<16)

	binaryPath := filepath.Join(t.TempDir(), "App")
	if err := os.WriteFile(binaryPath, data, 0755); err != nil {
		t.Fatal(err)
	}
	patched, changed, err := patchMachOMinimumOS(binaryPath, 16<<16)
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("expected LC_BUILD_VERSION minimum OS to be patched")
	}
	if got := binary.LittleEndian.Uint32(patched[44:48]); got != 16<<16 {
		t.Fatalf("patched minimum OS = %#x, want %#x", got, 16<<16)
	}
}

func TestPrepareIPACompatibilityAndPruning(t *testing.T) {
	ditto, err := exec.LookPath("ditto")
	if err != nil {
		t.Skip("ditto is only available on macOS")
	}
	t.Setenv("APPMONITOR_COMPATIBLE_EXTENSION_POINTS", "com.apple.widget-extension")
	inputPath := filepath.Join(t.TempDir(), "sample.ipa")
	createTestIPA(t, inputPath)

	firstOutput := filepath.Join(t.TempDir(), "sample-patched.ipa")
	first, err := PrepareIPA(context.Background(), inputPath, firstOutput, "16.0", false, IPATools{DittoPath: ditto})
	if err != nil {
		t.Fatal(err)
	}
	if !first.Changed || !first.HasIncompatibleExtensions {
		t.Fatalf("first preparation = %+v, want changed with incompatible extensions", first)
	}
	assertPlistMinimum(t, first.OutputPath, "Payload/Sample.app/Info.plist", "16.0")
	assertPlistMinimum(t, first.OutputPath, "Payload/Sample.app/PlugIns/Widget.appex/Info.plist", "16.0")

	prunedOutput := filepath.Join(t.TempDir(), "sample-pruned.ipa")
	pruned, err := PrepareIPA(context.Background(), inputPath, prunedOutput, "16.0", true, IPATools{DittoPath: ditto})
	if err != nil {
		t.Fatal(err)
	}
	if !pruned.Changed {
		t.Fatal("expected pruning to produce a changed IPA")
	}
	assertZipEntryAbsent(t, pruned.OutputPath, "Payload/Sample.app/PlugIns/Unsupported.appex/Info.plist")
	assertZipEntryPresent(t, pruned.OutputPath, "Payload/Sample.app/PlugIns/Widget.appex/Info.plist")
}

func createTestIPA(t *testing.T, path string) {
	t.Helper()
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	writer := zip.NewWriter(file)
	entries := map[string]any{
		"Payload/Sample.app/Info.plist": map[string]any{
			"CFBundleExecutable": "Sample",
			"MinimumOSVersion":   "18.0",
		},
		"Payload/Sample.app/PlugIns/Widget.appex/Info.plist": map[string]any{
			"CFBundleExecutable": "Widget",
			"MinimumOSVersion":   "17.0",
			"NSExtension": map[string]any{
				"NSExtensionPointIdentifier": "com.apple.widget-extension",
			},
		},
		"Payload/Sample.app/PlugIns/Unsupported.appex/Info.plist": map[string]any{
			"CFBundleExecutable": "Unsupported",
			"MinimumOSVersion":   "17.0",
			"NSExtension": map[string]any{
				"NSExtensionPointIdentifier": "com.example.unknown-extension",
			},
		},
	}
	entries["Payload/Sample.app/Sample"] = []byte("not a Mach-O executable")
	entries["Payload/Sample.app/PlugIns/Widget.appex/Widget"] = []byte("not a Mach-O executable")
	entries["Payload/Sample.app/PlugIns/Unsupported.appex/Unsupported"] = []byte("not a Mach-O executable")
	for name, value := range entries {
		header := &zip.FileHeader{Name: name, Method: zip.Deflate}
		entry, err := writer.CreateHeader(header)
		if err != nil {
			t.Fatal(err)
		}
		switch value := value.(type) {
		case map[string]any:
			data, err := plist.Marshal(value, plist.XMLFormat)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := entry.Write(data); err != nil {
				t.Fatal(err)
			}
		case []byte:
			if _, err := entry.Write(value); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
}

func assertPlistMinimum(t *testing.T, ipaPath, entryPath, expected string) {
	t.Helper()
	archive, err := zip.OpenReader(ipaPath)
	if err != nil {
		t.Fatal(err)
	}
	defer archive.Close()
	for _, entry := range archive.File {
		if entry.Name != entryPath {
			continue
		}
		reader, err := entry.Open()
		if err != nil {
			t.Fatal(err)
		}
		defer reader.Close()
		data, err := io.ReadAll(reader)
		if err != nil {
			t.Fatal(err)
		}
		var info map[string]any
		if _, err := plist.Unmarshal(data, &info); err != nil {
			t.Fatal(err)
		}
		if got := info["MinimumOSVersion"]; got != expected {
			t.Fatalf("%s MinimumOSVersion = %#v, want %q", entryPath, got, expected)
		}
		return
	}
	t.Fatalf("IPA is missing entry %s", entryPath)
}

func assertZipEntryPresent(t *testing.T, ipaPath, wanted string) {
	t.Helper()
	assertZipEntry(t, ipaPath, wanted, true)
}

func assertZipEntryAbsent(t *testing.T, ipaPath, wanted string) {
	t.Helper()
	assertZipEntry(t, ipaPath, wanted, false)
}

func assertZipEntry(t *testing.T, ipaPath, wanted string, present bool) {
	t.Helper()
	archive, err := zip.OpenReader(ipaPath)
	if err != nil {
		t.Fatal(err)
	}
	defer archive.Close()
	for _, entry := range archive.File {
		if entry.Name == wanted {
			if !present {
				t.Fatalf("unexpected IPA entry %s", wanted)
			}
			return
		}
	}
	if present {
		t.Fatalf("IPA is missing entry %s", wanted)
	}
}
