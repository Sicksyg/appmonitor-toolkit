package ios

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"

	"howett.net/plist"
)

const maxMachOHeaderSize = 8 << 20

// fastPatchIPA rewrites only the Info.plist files of an IPA by copying every other
// entry as-is, without extracting or recompressing. It applies when every bundle
// executable is FairPlay-encrypted (and so is left untouched anyway); otherwise it
// reports ok=false and the caller falls back to the full extract/patch/repack path.
func fastPatchIPA(inputPath, outputPath string, bundles []ipaBundle, target uint32) (IPAResult, bool, error) {
	archive, err := zip.OpenReader(inputPath)
	if err != nil {
		return IPAResult{}, false, fmt.Errorf("open IPA archive: %w", err)
	}
	defer archive.Close()

	entries := make(map[string]*zip.File, len(archive.File))
	for _, file := range archive.File {
		entries[path.Clean(file.Name)] = file
	}

	replacements := map[string][]byte{}
	for _, bundle := range bundles {
		infoFile := entries[bundle.infoPath]
		if infoFile == nil {
			return IPAResult{}, false, nil
		}
		data, err := readZipEntry(infoFile)
		if err != nil {
			return IPAResult{}, false, fmt.Errorf("read %s: %w", bundle.infoPath, err)
		}
		var info map[string]any
		if _, err := plist.Unmarshal(data, &info); err != nil {
			return IPAResult{}, false, fmt.Errorf("parse %s: %w", bundle.infoPath, err)
		}
		minimum, ok := info["MinimumOSVersion"].(string)
		needsPatch := !ok
		if ok {
			if needsPatch, err = versionGreaterThanTarget(minimum, target); err != nil {
				return IPAResult{}, false, fmt.Errorf("parse MinimumOSVersion %q: %w", minimum, err)
			}
		}
		if needsPatch {
			info["MinimumOSVersion"] = formatPackedVersion(target)
			patched, err := plist.Marshal(info, plist.BinaryFormat)
			if err != nil {
				return IPAResult{}, false, fmt.Errorf("encode patched Info.plist: %w", err)
			}
			replacements[bundle.infoPath] = patched
		}

		executable := infoString(info, "CFBundleExecutable")
		if executable == "" {
			continue
		}
		binaryFile := entries[path.Join(bundle.relativePath, executable)]
		if binaryFile == nil {
			continue
		}
		encrypted, err := zipMachOEncrypted(binaryFile)
		if err != nil || !encrypted {
			return IPAResult{}, false, nil
		}
	}

	if len(replacements) == 0 {
		return IPAResult{OutputPath: inputPath}, true, nil
	}
	if err := os.MkdirAll(filepath.Dir(outputPath), 0755); err != nil {
		return IPAResult{}, false, fmt.Errorf("create output directory: %w", err)
	}
	if err := writePatchedZip(archive, outputPath, replacements); err != nil {
		os.Remove(outputPath)
		return IPAResult{}, false, err
	}
	return IPAResult{OutputPath: outputPath, Changed: true}, true, nil
}

func zipMachOEncrypted(file *zip.File) (bool, error) {
	reader, err := file.Open()
	if err != nil {
		return false, err
	}
	defer reader.Close()
	header := make([]byte, 32)
	if _, err := io.ReadFull(reader, header); err != nil {
		return false, err
	}
	size := uint64(32) + uint64(uint32(header[20])|uint32(header[21])<<8|uint32(header[22])<<16|uint32(header[23])<<24)
	if size > maxMachOHeaderSize {
		return false, fmt.Errorf("Mach-O header too large")
	}
	rest := make([]byte, size-32)
	if _, err := io.ReadFull(reader, rest); err != nil {
		return false, err
	}
	return machOHeaderEncrypted(append(header, rest...))
}

func writePatchedZip(archive *zip.ReadCloser, outputPath string, replacements map[string][]byte) error {
	out, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("create output IPA: %w", err)
	}
	writer := zip.NewWriter(out)
	for _, file := range archive.File {
		if data, ok := replacements[path.Clean(file.Name)]; ok {
			header := file.FileHeader
			header.Method = zip.Deflate
			header.CompressedSize64, header.UncompressedSize64, header.CRC32 = 0, 0, 0
			w, err := writer.CreateHeader(&header)
			if err == nil {
				_, err = w.Write(data)
			}
			if err != nil {
				out.Close()
				return fmt.Errorf("write %s: %w", file.Name, err)
			}
			continue
		}
		if err := writer.Copy(file); err != nil {
			out.Close()
			return fmt.Errorf("copy %s: %w", file.Name, err)
		}
	}
	if err := writer.Close(); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}
