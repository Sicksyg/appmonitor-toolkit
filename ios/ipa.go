package ios

import (
	"archive/zip"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"howett.net/plist"
)

const maxInfoPlistSize = 32 << 20

var compatibleExtensionPoints = map[string]struct{}{
	"com.apple.action":                  {},
	"com.apple.AppSSO.idp-extension":    {},
	"com.apple.AudioUnit-UI":            {},
	"com.apple.authentication-services": {},
	"com.apple.authentication-services-account-authentication-modification-ui": {},
	"com.apple.authentication-services-credential-provider-ui":                 {},
	"com.apple.broadcast-services-setupui":                                     {},
	"com.apple.broadcast-services-upload":                                      {},
	"com.apple.callkit.call-directory":                                         {},
	"com.apple.classkit.context-provider":                                      {},
	"com.apple.document-provider":                                              {},
	"com.apple.document-provider.file":                                         {},
	"com.apple.fileprovider":                                                   {},
	"com.apple.fileprovider-actionsui":                                         {},
	"com.apple.fileprovider-nonui":                                             {},
	"com.apple.identitylookup.classification-ui":                               {},
	"com.apple.identitylookup.message-filter":                                  {},
	"com.apple.intents-service":                                                {},
	"com.apple.intents-ui-service":                                             {},
	"com.apple.keyboard-service":                                               {},
	"com.apple.message-payload-provider":                                       {},
	"com.apple.networkextension.app-proxy":                                     {},
	"com.apple.networkextension.app-push":                                      {},
	"com.apple.networkextension.filter-control":                                {},
	"com.apple.networkextension.filter-data":                                   {},
	"com.apple.networkextension.packet-tunnel":                                 {},
	"com.apple.notificationcenter.widget":                                      {},
	"com.apple.photo-editing":                                                  {},
	"com.apple.photo-project":                                                  {},
	"com.apple.quicklook.preview":                                              {},
	"com.apple.quicklook.thumbnail":                                            {},
	"com.apple.Safari.content-blocker":                                         {},
	"com.apple.Safari.extension":                                               {},
	"com.apple.services":                                                       {},
	"com.apple.share-services":                                                 {},
	"com.apple.sirikit.intents":                                                {},
	"com.apple.sirikit.intentsui":                                              {},
	"com.apple.spotlight.index":                                                {},
	"com.apple.ui-services":                                                    {},
	"com.apple.usernotifications.content-extension":                            {},
	"com.apple.usernotifications.service":                                      {},
	"com.apple.widget-extension":                                               {},
	"com.apple.widgetkit-extension":                                            {},
}

// IPA tools used for archive handling and executable signing.
type IPATools struct {
	DittoPath string
	LDIDPath  string
	LibPath   string
}

// IPAResult describes the IPA produced by compatibility preparation.
type IPAResult struct {
	OutputPath                string
	Changed                   bool
	HasIncompatibleExtensions bool
}

func (m *Manager) ipaTools() IPATools {
	return IPATools{DittoPath: m.dittoPath, LDIDPath: m.ldidPath, LibPath: m.libPath}
}

func ipaOutputPath(directory, bundleID, suffix string) string {
	base := strings.Map(func(char rune) rune {
		switch {
		case char >= 'a' && char <= 'z',
			char >= 'A' && char <= 'Z',
			char >= '0' && char <= '9',
			char == '.' || char == '-' || char == '_':
			return char
		default:
			return '_'
		}
	}, bundleID)
	return filepath.Join(directory, base+suffix+".ipa")
}

type ipaBundle struct {
	relativePath string
	infoPath     string
	executable   string
	extension    bool
	point        string
}

// PrepareIPA lowers declared and Mach-O minimum OS versions, and optionally
// removes extension bundles outside the compatibility allowlist.
func PrepareIPA(ctx context.Context, inputPath, outputPath, targetVersion string, prune bool, tools IPATools) (IPAResult, error) {
	target, err := packedVersion(targetVersion)
	if err != nil {
		return IPAResult{}, fmt.Errorf("invalid target iOS version %q: %w", targetVersion, err)
	}
	if inputPath == "" || outputPath == "" {
		return IPAResult{}, fmt.Errorf("input and output IPA paths are required")
	}
	inputAbs, err := filepath.Abs(inputPath)
	if err != nil {
		return IPAResult{}, fmt.Errorf("resolve input IPA path: %w", err)
	}
	outputAbs, err := filepath.Abs(outputPath)
	if err != nil {
		return IPAResult{}, fmt.Errorf("resolve output IPA path: %w", err)
	}
	if inputAbs == outputAbs {
		return IPAResult{}, fmt.Errorf("output IPA path must differ from input")
	}

	bundles, appPath, err := inspectIPABundles(inputAbs)
	if err != nil {
		return IPAResult{}, err
	}
	hasIncompatible := false
	for _, bundle := range bundles {
		if bundle.extension && !isCompatibleExtensionPoint(bundle.point) {
			hasIncompatible = true
		}
	}

	destination, err := os.MkdirTemp("", "appmonitor-ipa-*")
	if err != nil {
		return IPAResult{}, fmt.Errorf("create IPA working directory: %w", err)
	}
	defer os.RemoveAll(destination)

	dittoPath, err := resolveDitto(tools.DittoPath)
	if err != nil {
		return IPAResult{}, err
	}
	if err := runDitto(ctx, dittoPath, "extract IPA", "", "-x", "-k", inputAbs, destination); err != nil {
		return IPAResult{}, err
	}

	extractedAppPath := filepath.Join(destination, filepath.FromSlash(appPath))
	changed := false
	for _, bundle := range bundles {
		bundlePath := filepath.Join(destination, filepath.FromSlash(bundle.relativePath))
		if bundle.extension && prune && !isCompatibleExtensionPoint(bundle.point) {
			if err := os.RemoveAll(bundlePath); err != nil {
				return IPAResult{}, fmt.Errorf("remove incompatible extension %q: %w", bundle.relativePath, err)
			}
			changed = true
			continue
		}
		bundleChanged, err := patchBundle(ctx, bundlePath, target, tools.LDIDPath, tools.LibPath, destination)
		if err != nil {
			return IPAResult{}, fmt.Errorf("prepare bundle %q: %w", bundle.relativePath, err)
		}
		changed = changed || bundleChanged
	}

	if !changed {
		return IPAResult{
			OutputPath:                inputAbs,
			HasIncompatibleExtensions: hasIncompatible,
		}, nil
	}
	if _, err := os.Stat(extractedAppPath); err != nil {
		return IPAResult{}, fmt.Errorf("expected app bundle after IPA extraction: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(outputAbs), 0755); err != nil {
		return IPAResult{}, fmt.Errorf("create output directory: %w", err)
	}
	if err := os.Remove(outputAbs); err != nil && !os.IsNotExist(err) {
		return IPAResult{}, fmt.Errorf("remove previous output IPA: %w", err)
	}
	if err := runDitto(ctx, dittoPath, "repackage IPA", destination, "-c", "-k", "--sequesterRsrc", "--keepParent", "Payload", outputAbs); err != nil {
		return IPAResult{}, err
	}
	return IPAResult{
		OutputPath:                outputAbs,
		Changed:                   true,
		HasIncompatibleExtensions: hasIncompatible,
	}, nil
}

func inspectIPABundles(ipaPath string) ([]ipaBundle, string, error) {
	archive, err := zip.OpenReader(ipaPath)
	if err != nil {
		return nil, "", fmt.Errorf("open IPA archive: %w", err)
	}
	defer archive.Close()

	var mainApp string
	var mainInfo string
	for _, file := range archive.File {
		entry := path.Clean(file.Name)
		if strings.HasPrefix(entry, "Payload/") && strings.Count(entry, "/") == 2 &&
			strings.HasSuffix(entry, ".app/Info.plist") {
			if mainInfo != "" {
				return nil, "", fmt.Errorf("IPA contains multiple app bundles under Payload")
			}
			mainInfo = entry
			mainApp = path.Dir(entry)
		}
	}
	if mainInfo == "" {
		return nil, "", fmt.Errorf("IPA does not contain Payload/<app>.app/Info.plist")
	}

	var bundles []ipaBundle
	main, err := readIPABundle(archive.File, mainApp, mainInfo, false)
	if err != nil {
		return nil, "", err
	}
	bundles = append(bundles, main)
	for _, file := range archive.File {
		entry := path.Clean(file.Name)
		if !strings.HasPrefix(entry, mainApp+"/") || !strings.HasSuffix(entry, ".appex/Info.plist") {
			continue
		}
		relative := strings.TrimPrefix(entry, mainApp+"/")
		parts := strings.Split(relative, "/")
		if len(parts) != 3 || (parts[0] != "PlugIns" && parts[0] != "Extensions") {
			continue
		}
		appexPath := path.Join(mainApp, parts[0], parts[1])
		bundle, err := readIPABundle(archive.File, appexPath, entry, true)
		if err != nil {
			return nil, "", err
		}
		bundles = append(bundles, bundle)
	}
	return bundles, mainApp, nil
}

func readIPABundle(files []*zip.File, bundlePath, infoPath string, extension bool) (ipaBundle, error) {
	var infoFile *zip.File
	for _, file := range files {
		if path.Clean(file.Name) == infoPath {
			infoFile = file
			break
		}
	}
	if infoFile == nil {
		return ipaBundle{}, fmt.Errorf("IPA is missing %s", infoPath)
	}
	data, err := readZipEntry(infoFile)
	if err != nil {
		return ipaBundle{}, fmt.Errorf("read %s: %w", infoPath, err)
	}
	var info map[string]any
	if _, err := plist.Unmarshal(data, &info); err != nil {
		return ipaBundle{}, fmt.Errorf("parse %s: %w", infoPath, err)
	}
	bundle := ipaBundle{
		relativePath: bundlePath,
		infoPath:     path.Join(bundlePath, "Info.plist"),
		extension:    extension,
	}
	if executable, ok := info["CFBundleExecutable"].(string); ok {
		bundle.executable = executable
	}
	if extension {
		bundle.point = extensionPoint(info)
	}
	return bundle, nil
}

func readZipEntry(file *zip.File) ([]byte, error) {
	if file.UncompressedSize64 > maxInfoPlistSize {
		return nil, fmt.Errorf("property list exceeds %d bytes", maxInfoPlistSize)
	}
	reader, err := file.Open()
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	return io.ReadAll(io.LimitReader(reader, maxInfoPlistSize+1))
}

func extensionPoint(info map[string]any) string {
	for _, key := range []string{"EXAppExtensionAttributes", "NSExtension"} {
		nested, ok := info[key].(map[string]any)
		if !ok {
			continue
		}
		if point, ok := nested["EXExtensionPointIdentifier"].(string); ok && point != "" {
			return point
		}
		if point, ok := nested["NSExtensionPointIdentifier"].(string); ok && point != "" {
			return point
		}
	}
	return "UNKNOWN"
}

func isCompatibleExtensionPoint(point string) bool {
	if configured, exists := os.LookupEnv("APPMONITOR_COMPATIBLE_EXTENSION_POINTS"); exists {
		custom := strings.Fields(configured)
		for _, allowed := range custom {
			if point == allowed {
				return true
			}
		}
		return false
	}
	_, ok := compatibleExtensionPoints[point]
	return ok
}

func patchBundle(ctx context.Context, bundlePath string, target uint32, ldidPath, libPath, workDir string) (bool, error) {
	infoPath := filepath.Join(bundlePath, "Info.plist")
	infoData, err := os.ReadFile(infoPath)
	if err != nil {
		return false, fmt.Errorf("read Info.plist: %w", err)
	}
	var info map[string]any
	if _, err := plist.Unmarshal(infoData, &info); err != nil {
		return false, fmt.Errorf("parse Info.plist: %w", err)
	}
	changed := false
	minimum, ok := info["MinimumOSVersion"].(string)
	needsVersionPatch := !ok
	if ok {
		needsVersionPatch, err = versionGreaterThanTarget(minimum, target)
		if err != nil {
			return false, fmt.Errorf("parse MinimumOSVersion %q: %w", minimum, err)
		}
	}
	if needsVersionPatch {
		info["MinimumOSVersion"] = formatPackedVersion(target)
		patched, err := plist.Marshal(info, plist.BinaryFormat)
		if err != nil {
			return false, fmt.Errorf("encode patched Info.plist: %w", err)
		}
		if err := os.WriteFile(infoPath, patched, 0644); err != nil {
			return false, fmt.Errorf("write patched Info.plist: %w", err)
		}
		changed = true
	}

	executable := strings.TrimSpace(infoString(info, "CFBundleExecutable"))
	if executable == "" {
		return changed, nil
	}
	cleanExecutable := filepath.Clean(executable)
	if filepath.IsAbs(executable) || cleanExecutable == "." || cleanExecutable == ".." ||
		strings.HasPrefix(cleanExecutable, ".."+string(filepath.Separator)) {
		return false, fmt.Errorf("unsafe CFBundleExecutable path %q", executable)
	}
	executablePath := filepath.Join(bundlePath, cleanExecutable)
	if _, err := os.Stat(executablePath); os.IsNotExist(err) {
		return changed, nil
	} else if err != nil {
		return false, fmt.Errorf("inspect executable %q: %w", executable, err)
	}
	patched, binaryChanged, err := patchMachOMinimumOS(executablePath, target)
	if err != nil {
		return false, err
	}
	if !binaryChanged {
		return changed, nil
	}
	if err := os.WriteFile(executablePath, patched, 0755); err != nil {
		return false, fmt.Errorf("write patched executable: %w", err)
	}
	if err := resignBinary(ctx, ldidPath, libPath, executablePath, workDir); err != nil {
		return false, err
	}
	return true, nil
}

func infoString(info map[string]any, key string) string {
	value, _ := info[key].(string)
	return value
}

func patchMachOMinimumOS(binaryPath string, target uint32) ([]byte, bool, error) {
	data, err := os.ReadFile(binaryPath)
	if err != nil {
		return nil, false, fmt.Errorf("read executable: %w", err)
	}
	if len(data) < 32 || binary.LittleEndian.Uint32(data[:4]) != 0xfeedfacf ||
		binary.LittleEndian.Uint32(data[4:8]) != 0x0100000c {
		return data, false, nil
	}
	ncmds := binary.LittleEndian.Uint32(data[16:20])
	offset := uint64(32)
	for index := uint32(0); index < ncmds; index++ {
		if offset+8 > uint64(len(data)) {
			return nil, false, fmt.Errorf("malformed Mach-O load command table")
		}
		command := binary.LittleEndian.Uint32(data[offset : offset+4])
		commandSize := binary.LittleEndian.Uint32(data[offset+4 : offset+8])
		if commandSize < 8 || offset+uint64(commandSize) > uint64(len(data)) {
			return nil, false, fmt.Errorf("malformed Mach-O load command size")
		}
		var versionOffset uint64
		switch command {
		case 0x32:
			if commandSize < 24 {
				return nil, false, fmt.Errorf("malformed LC_BUILD_VERSION command")
			}
			versionOffset = offset + 12
		case 0x25:
			if commandSize < 16 {
				return nil, false, fmt.Errorf("malformed LC_VERSION_MIN_IPHONEOS command")
			}
			versionOffset = offset + 8
		}
		if versionOffset == 0 {
			offset += uint64(commandSize)
			continue
		}
		current := binary.LittleEndian.Uint32(data[versionOffset : versionOffset+4])
		if current <= target {
			return data, false, nil
		}
		binary.LittleEndian.PutUint32(data[versionOffset:versionOffset+4], target)
		return data, true, nil
	}
	return data, false, nil
}

func resignBinary(ctx context.Context, configuredPath, libPath, binaryPath, workDir string) error {
	ldidPath, err := resolveLDID(configuredPath)
	if err != nil {
		return err
	}
	entitlements := filepath.Join(workDir, "entitlements-"+strconv.Itoa(os.Getpid())+".plist")
	extract := ldidCommand(ctx, ldidPath, libPath, "-e", binaryPath)
	entitlementData, extractErr := extract.Output()
	if extractErr == nil && len(entitlementData) > 0 {
		if err := os.WriteFile(entitlements, entitlementData, 0600); err != nil {
			return fmt.Errorf("save executable entitlements: %w", err)
		}
		defer os.Remove(entitlements)
		if err := runLDID(ctx, ldidPath, libPath, "-S"+entitlements, binaryPath); err != nil {
			return fmt.Errorf("re-sign executable preserving entitlements: %w", err)
		}
		return nil
	}
	if err := runLDID(ctx, ldidPath, libPath, "-S", binaryPath); err != nil {
		return fmt.Errorf("re-sign executable: %w", err)
	}
	return nil
}

func resolveLDID(configuredPath string) (string, error) {
	if configuredPath != "" {
		info, err := os.Stat(configuredPath)
		if err == nil && !info.IsDir() {
			return configuredPath, nil
		}
		if strings.TrimSpace(os.Getenv("APPMONITOR_LDID_PATH")) != "" {
			return "", fmt.Errorf("configured ldid executable %q is unavailable", configuredPath)
		}
	}
	path, err := exec.LookPath("ldid")
	if err != nil {
		return "", fmt.Errorf("Mach-O minimum-OS patching requires ldid; bundle it at the configured tools path or add it to PATH")
	}
	return path, nil
}

func resolveDitto(configuredPath string) (string, error) {
	if configuredPath != "" {
		if info, err := os.Stat(configuredPath); err == nil && !info.IsDir() {
			return configuredPath, nil
		}
		return "", fmt.Errorf("configured ditto executable %q is unavailable", configuredPath)
	}
	path, err := exec.LookPath("ditto")
	if err != nil {
		return "", fmt.Errorf("ditto is required to extract and repackage iOS IPAs")
	}
	return path, nil
}

func runDitto(ctx context.Context, dittoPath, operation, workingDirectory string, args ...string) error {
	cmd := exec.CommandContext(ctx, dittoPath, args...)
	if workingDirectory != "" {
		cmd.Dir = workingDirectory
	}
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %w: %s", operation, err, strings.TrimSpace(string(output)))
	}
	return nil
}

func ldidCommand(ctx context.Context, executable, libPath string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, executable, args...)
	if libPath != "" {
		architectureLibPath := filepath.Join(libPath, "darwin-"+runtime.GOARCH)
		libraryPaths := architectureLibPath + string(os.PathListSeparator) + libPath
		cmd.Env = append(os.Environ(),
			"DYLD_LIBRARY_PATH="+libraryPaths,
			"DYLD_FALLBACK_LIBRARY_PATH="+libraryPaths,
		)
	}
	return cmd
}

func runLDID(ctx context.Context, executable, libPath string, args ...string) error {
	cmd := ldidCommand(ctx, executable, libPath, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}

func packedVersion(value string) (uint32, error) {
	parts := strings.Split(value, ".")
	if len(parts) == 0 || len(parts) > 3 {
		return 0, fmt.Errorf("expected one to three numeric components")
	}
	var components [3]uint32
	for index, part := range parts {
		component, err := strconv.ParseUint(part, 10, 16)
		if err != nil || component > 255 && index > 0 {
			return 0, fmt.Errorf("invalid version component %q", part)
		}
		components[index] = uint32(component)
	}
	return components[0]<<16 | components[1]<<8 | components[2], nil
}

func versionGreaterThanTarget(value string, target uint32) (bool, error) {
	current, err := packedVersion(value)
	if err != nil {
		return false, err
	}
	return current > target, nil
}

func formatPackedVersion(value uint32) string {
	major, minor, patch := value>>16, (value>>8)&0xff, value&0xff
	if patch == 0 {
		return fmt.Sprintf("%d.%d", major, minor)
	}
	return fmt.Sprintf("%d.%d.%d", major, minor, patch)
}
