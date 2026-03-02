package util

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func CompilePlugin(srcDir, outputPath string) error {
	cmd := exec.Command("go", "build", "-buildmode=plugin", "-o", outputPath, ".")
	cmd.Dir = srcDir
	out, err := cmd.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if msg != "" {
			return fmt.Errorf("compilation failed: %w\n%s", err, msg)
		}
		return fmt.Errorf("compilation failed: %w", err)
	}
	return nil
}

func ExtractTarball(r io.Reader, destDir string) (string, error) {
	gz, err := gzip.NewReader(r)
	if err != nil {
		return "", fmt.Errorf("failed to decompress tarball: %w", err)
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	safeBase := filepath.Clean(destDir) + string(filepath.Separator)
	var topDir string

	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", fmt.Errorf("failed to read tarball: %w", err)
		}

		// Skip PAX extended-header entries (e.g. pax_global_header);
		// they carry metadata only and have no real path on disk.
		if header.Typeflag == tar.TypeXGlobalHeader || header.Typeflag == tar.TypeXHeader {
			continue
		}

		// Sanitize path to prevent zip-slip attacks.
		cleanName := filepath.Clean(header.Name)
		destPath := filepath.Join(destDir, cleanName)
		if !strings.HasPrefix(destPath+string(filepath.Separator), safeBase) {
			continue // skip unsafe entries
		}

		if topDir == "" {
			topDir = strings.SplitN(cleanName, string(filepath.Separator), 2)[0]
		}

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(destPath, 0o755); err != nil {
				return "", fmt.Errorf("failed to create directory %q: %w", destPath, err)
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
				return "", fmt.Errorf("failed to create parent directory: %w", err)
			}
			f, err := os.OpenFile(destPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(header.Mode)&0o755)
			if err != nil {
				return "", fmt.Errorf("failed to create file %q: %w", destPath, err)
			}
			_, copyErr := io.Copy(f, tr)
			f.Close()
			if copyErr != nil {
				return "", fmt.Errorf("failed to write file %q: %w", destPath, copyErr)
			}
		}
	}

	if topDir == "" {
		return "", fmt.Errorf("empty or invalid source tarball")
	}

	return filepath.Join(destDir, topDir), nil
}
