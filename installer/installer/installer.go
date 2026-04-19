package installer

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

// ProgressFunc is called periodically during download with bytes written and total bytes.
type ProgressFunc func(written, total int64)

// Release holds the resolved download URL and version tag for a GitHub release.
type Release struct {
	Version     string
	DownloadURL string
	AssetName   string
}

// ResolveLatest fetches the latest release from GitHub and returns the asset
// matching the current OS/ARCH.
func ResolveLatest(githubRepo, binaryName string) (*Release, error) {
	return Resolve(githubRepo, binaryName, "")
}

// Resolve fetches a specific release by version tag (e.g. "v0.0.4"), or the
// latest release when version is empty or "dev".
func Resolve(githubRepo, binaryName, version string) (*Release, error) {
	apiURL := fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", githubRepo)
	if version != "" && version != "dev" {
		apiURL = fmt.Sprintf("https://api.github.com/repos/%s/releases/tags/%s", githubRepo, version)
	}

	resp, err := http.Get(apiURL)
	if err != nil {
		return nil, fmt.Errorf("contacting GitHub API: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		if resp.StatusCode == http.StatusNotFound {
			return nil, fmt.Errorf("no releases found for %s (404)\nURL: %s\nBody: %s", githubRepo, apiURL, body)
		}
		return nil, fmt.Errorf("GitHub API returned %d\nURL: %s\nBody: %s", resp.StatusCode, apiURL, body)
	}

	var release struct {
		TagName string `json:"tag_name"`
		Assets  []struct {
			Name               string `json:"name"`
			BrowserDownloadURL string `json:"browser_download_url"`
		} `json:"assets"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, fmt.Errorf("decoding GitHub response: %w", err)
	}

	osSuffix, archSuffix := platformSuffixes()
	for _, asset := range release.Assets {
		name := strings.ToLower(asset.Name)
		if strings.Contains(name, strings.ToLower(binaryName)) &&
			strings.Contains(name, osSuffix) &&
			strings.Contains(name, archSuffix) {
			return &Release{
				Version:     release.TagName,
				DownloadURL: asset.BrowserDownloadURL,
				AssetName:   asset.Name,
			}, nil
		}
	}

	return nil, fmt.Errorf(
		"no asset found for %s/%s in release %s",
		runtime.GOOS, runtime.GOARCH, release.TagName,
	)
}

// DownloadBinary downloads the release asset to a temp file and returns its path.
func DownloadBinary(release *Release, progress ProgressFunc) (string, error) {
	resp, err := http.Get(release.DownloadURL)
	if err != nil {
		return "", fmt.Errorf("downloading binary: %w", err)
	}
	defer resp.Body.Close()

	tmp, err := os.CreateTemp("", "mycli-setup-*")
	if err != nil {
		return "", fmt.Errorf("creating temp file: %w", err)
	}
	defer tmp.Close()

	var written int64
	total := resp.ContentLength
	buf := make([]byte, 32*1024)

	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			if _, werr := tmp.Write(buf[:n]); werr != nil {
				return "", fmt.Errorf("writing temp file: %w", werr)
			}
			written += int64(n)
			if progress != nil {
				progress(written, total)
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", fmt.Errorf("reading download stream: %w", err)
		}
	}

	return tmp.Name(), nil
}

// InstallBinary copies (or extracts) the binary from src to destDir/binaryName.
// If srcPath is a .tar.gz archive it extracts the first entry whose name matches
// binaryName; otherwise it copies the file directly.
func InstallBinary(srcPath, destDir, binaryName string) (string, error) {
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return "", fmt.Errorf("creating install directory: %w", err)
	}

	name := binaryName
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	destPath := filepath.Join(destDir, name)

	dst, err := os.OpenFile(destPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
	if err != nil {
		return "", fmt.Errorf("creating destination file (permission issue?): %w", err)
	}
	defer dst.Close()

	src, err := os.Open(srcPath)
	if err != nil {
		return "", fmt.Errorf("opening source file: %w", err)
	}
	defer src.Close()

	if isGzip(srcPath) {
		if err := extractBinaryFromTarGz(src, dst, name); err != nil {
			return "", err
		}
		return destPath, nil
	}

	if isZip(srcPath) {
		src.Close()
		if err := extractBinaryFromZip(srcPath, dst, name); err != nil {
			return "", err
		}
		return destPath, nil
	}

	if _, err := io.Copy(dst, src); err != nil {
		return "", fmt.Errorf("copying binary: %w", err)
	}
	return destPath, nil
}

func isGzip(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	magic := make([]byte, 2)
	n, _ := f.Read(magic)
	return n == 2 && magic[0] == 0x1f && magic[1] == 0x8b
}

func isZip(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	magic := make([]byte, 4)
	n, _ := f.Read(magic)
	return n == 4 && magic[0] == 0x50 && magic[1] == 0x4b && magic[2] == 0x03 && magic[3] == 0x04
}

func extractBinaryFromZip(srcPath string, dst io.Writer, binaryName string) error {
	zr, err := zip.OpenReader(srcPath)
	if err != nil {
		return fmt.Errorf("opening zip: %w", err)
	}
	defer zr.Close()

	for _, f := range zr.File {
		if filepath.Base(f.Name) == binaryName {
			rc, err := f.Open()
			if err != nil {
				return fmt.Errorf("opening zip entry: %w", err)
			}
			defer rc.Close()
			if _, err := io.Copy(dst, rc); err != nil {
				return fmt.Errorf("extracting binary from zip: %w", err)
			}
			return nil
		}
	}
	return fmt.Errorf("binary %q not found in zip archive", binaryName)
}

func extractBinaryFromTarGz(r io.Reader, dst io.Writer, binaryName string) error {
	gz, err := gzip.NewReader(r)
	if err != nil {
		return fmt.Errorf("opening gzip stream: %w", err)
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("reading tar: %w", err)
		}
		if hdr.Typeflag != tar.TypeReg {
			continue
		}
		if filepath.Base(hdr.Name) == binaryName {
			if _, err := io.Copy(dst, tr); err != nil {
				return fmt.Errorf("extracting binary: %w", err)
			}
			return nil
		}
	}
	return fmt.Errorf("binary %q not found in archive", binaryName)
}

// WriteEnvFile writes a .env file containing the given env var name=value pairs to dir/.env.
// The file is created with mode 0600 (owner-readable only).
func WriteEnvFile(dir string, envVars map[string]string) (string, error) {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("creating directory: %w", err)
	}

	names := make([]string, 0, len(envVars))
	for k := range envVars {
		names = append(names, k)
	}
	sort.Strings(names)

	var sb strings.Builder
	for _, name := range names {
		sb.WriteString(name)
		sb.WriteString("=")
		sb.WriteString(envVars[name])
		sb.WriteString("\n")
	}

	envPath := filepath.Join(dir, ".env")
	if err := os.WriteFile(envPath, []byte(sb.String()), 0600); err != nil {
		return "", fmt.Errorf("writing .env file: %w", err)
	}
	return envPath, nil
}

// WriteConfig writes the provided TOML content to configDir/configFileName.
func WriteConfig(configDir, configFileName, content string) (string, error) {
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return "", fmt.Errorf("creating config directory: %w", err)
	}

	configPath := filepath.Join(configDir, configFileName)
	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		return "", fmt.Errorf("writing config file: %w", err)
	}

	return configPath, nil
}

// CopyConfig copies an existing config file to destDir/configFileName.
func CopyConfig(srcPath, destDir, configFileName string) error {
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return fmt.Errorf("creating directory: %w", err)
	}

	data, err := os.ReadFile(srcPath)
	if err != nil {
		return fmt.Errorf("reading config: %w", err)
	}

	destPath := filepath.Join(destDir, configFileName)
	if err := os.WriteFile(destPath, data, 0644); err != nil {
		return fmt.Errorf("copying config: %w", err)
	}

	return nil
}

// DefaultInstallDir returns a sensible default install directory for the current OS.
func DefaultInstallDir() string {
	switch runtime.GOOS {
	case "windows":
		if p := os.Getenv("LOCALAPPDATA"); p != "" {
			return filepath.Join(p, "Programs", "bin")
		}
		return `C:\Program Files\bin`
	default:
		home, _ := os.UserHomeDir()
		return filepath.Join(home, ".local", "bin")
	}
}

// DefaultConfigDir returns a sensible default config directory for the current OS.
func DefaultConfigDir(appName string) string {
	switch runtime.GOOS {
	case "windows":
		if p := os.Getenv("APPDATA"); p != "" {
			return filepath.Join(p, appName)
		}
		return filepath.Join(`C:\ProgramData`, appName)
	default:
		home, _ := os.UserHomeDir()
		return filepath.Join(home, ".config", appName)
	}
}

// platformSuffixes returns the lowercase OS and arch substrings used in GoReleaser asset names.
// GoReleaser default naming: {binary}_{version}_{os}_{arch}.{ext}
func platformSuffixes() (osName, arch string) {
	return strings.ToLower(runtime.GOOS), strings.ToLower(runtime.GOARCH)
}
