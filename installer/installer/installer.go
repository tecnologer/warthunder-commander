package installer

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/tecnologer/warthunder/internal/utils/closer"
)

// ProgressFunc is called periodically during download with bytes written and total bytes.
type ProgressFunc func(written, total int64)

// Release holds the resolved download URL and version tag for a GitHub release.
type Release struct {
	Version     string
	DownloadURL string
	AssetName   string
}

type githubRelease struct {
	TagName string `json:"tag_name"`
	Assets  []struct {
		Name               string `json:"name"`
		BrowserDownloadURL string `json:"browser_download_url"`
	} `json:"assets"`
}

const (
	githubAPIURL            = "https://api.github.com"
	httpPlaceholderURL      = "https://example.invalid"
	windowsOS               = "windows"
	dirPerm                 = 0o755
	readWriteFilePerm       = 0o644
	privateFilePerm         = 0o600
	executableFilePerm      = 0o755
	maxExtractedBinaryBytes = int64(512 << 20) // 512 MiB safety cap for archive extraction.
)

// ResolveLatest fetches the latest release from GitHub and returns the asset
// matching the current OS/ARCH.
func ResolveLatest(githubRepo, binaryName string) (*Release, error) {
	return Resolve(githubRepo, binaryName, "")
}

// Resolve fetches a specific release by version tag (e.g. "v0.0.4"), or the
// latest release when version is empty or "dev".
func Resolve(githubRepo, binaryName, version string) (*Release, error) {
	releasePath := releaseAPIPath(githubRepo, version)

	release, err := fetchRelease(context.Background(), releasePath)
	if err != nil {
		return nil, err
	}

	return selectMatchingAsset(release, binaryName)
}

// DownloadBinary downloads the release asset to a temp file and returns its path.
func DownloadBinary(release *Release, progress ProgressFunc) (string, error) {
	resp, err := downloadReleaseResponse(context.Background(), release.DownloadURL)
	if err != nil {
		return "", err
	}

	defer closer.Close(resp.Body)

	return writeResponseToTemp(resp.Body, resp.ContentLength, progress)
}

func downloadReleaseResponse(ctx context.Context, downloadURL string) (*http.Response, error) {
	req, err := newValidatedGETRequest(ctx, downloadURL)
	if err != nil {
		return nil, fmt.Errorf("downloading binary: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("downloading binary: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		defer closer.Close(resp.Body)

		bodySnippet, readErr := io.ReadAll(io.LimitReader(resp.Body, 512))
		if readErr != nil {
			return nil, fmt.Errorf(
				"downloading binary: unexpected HTTP status %s and failed to read error response: %w",
				resp.Status,
				readErr,
			)
		}

		snippet := strings.TrimSpace(string(bodySnippet))
		if snippet != "" {
			return nil, fmt.Errorf(
				"downloading binary: unexpected HTTP status %s: %s",
				resp.Status,
				snippet,
			)
		}

		return nil, fmt.Errorf("downloading binary: unexpected HTTP status %s", resp.Status)
	}

	return resp, nil
}

func writeResponseToTemp(src io.Reader, total int64, progress ProgressFunc) (string, error) {
	tmp, err := os.CreateTemp("", "warthunder-setup-*")
	if err != nil {
		return "", fmt.Errorf("creating temp file: %w", err)
	}

	defer closer.Close(tmp)

	var (
		written int64
		buf     = make([]byte, 32*1024)
	)

	for {
		bytesRead, err := src.Read(buf)
		if bytesRead > 0 {
			if _, werr := tmp.Write(buf[:bytesRead]); werr != nil {
				return "", fmt.Errorf("writing temp file: %w", werr)
			}

			written += int64(bytesRead)

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
	if err := os.MkdirAll(destDir, dirPerm); err != nil {
		return "", fmt.Errorf("creating install directory: %w", err)
	}

	name := executableName(binaryName)

	destPath := filepath.Join(destDir, name)

	dst, err := os.OpenFile(destPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, readWriteFilePerm)
	if err != nil {
		return "", fmt.Errorf("creating destination file (permission issue?): %w", err)
	}

	defer closer.Close(dst)

	if err := writeInstalledBinary(srcPath, name, dst); err != nil {
		return "", err
	}

	if err := ensureExecutable(destPath); err != nil {
		return "", err
	}

	return destPath, nil
}

func executableName(binaryName string) string {
	if runtime.GOOS == windowsOS {
		return binaryName + ".exe"
	}

	return binaryName
}

func writeInstalledBinary(srcPath, binaryName string, dst io.Writer) error {
	switch {
	case isGzip(srcPath):
		if err := extractTarGzBinary(srcPath, binaryName, dst); err != nil {
			return fmt.Errorf("extracting tar.gz: %w", err)
		}
	case isZip(srcPath):
		if err := extractBinaryFromZip(srcPath, dst, binaryName); err != nil {
			return err
		}
	default:
		if err := copyBinaryFile(srcPath, dst); err != nil {
			return err
		}
	}

	return nil
}

func extractTarGzBinary(srcPath, binaryName string, dst io.Writer) error {
	src, err := os.Open(srcPath)
	if err != nil {
		return fmt.Errorf("opening source file: %w", err)
	}

	defer closer.Close(src)

	if err := extractBinaryFromTarGz(src, dst, binaryName); err != nil {
		return err
	}

	return nil
}

func copyBinaryFile(srcPath string, dst io.Writer) error {
	src, err := os.Open(srcPath)
	if err != nil {
		return fmt.Errorf("opening source file: %w", err)
	}

	defer closer.Close(src)

	if _, err := io.Copy(dst, src); err != nil {
		return fmt.Errorf("copying binary: %w", err)
	}

	return nil
}

func ensureExecutable(destPath string) error {
	if runtime.GOOS == windowsOS {
		return nil
	}

	if err := os.Chmod(destPath, executableFilePerm); err != nil {
		return fmt.Errorf("setting executable permissions: %w", err)
	}

	return nil
}

func isGzip(path string) bool {
	gzipFile, err := os.Open(path)
	if err != nil {
		return false
	}

	defer closer.Close(gzipFile)

	magic := make([]byte, 2)
	n, _ := gzipFile.Read(magic)

	return n == 2 && magic[0] == 0x1f && magic[1] == 0x8b
}

func isZip(path string) bool {
	zipFile, err := os.Open(path)
	if err != nil {
		return false
	}

	defer closer.Close(zipFile)

	magic := make([]byte, 4)
	n, _ := zipFile.Read(magic)

	return n == 4 && magic[0] == 0x50 && magic[1] == 0x4b && magic[2] == 0x03 && magic[3] == 0x04
}

func extractBinaryFromZip(srcPath string, dst io.Writer, binaryName string) error {
	zipReader, err := zip.OpenReader(srcPath)
	if err != nil {
		return fmt.Errorf("opening zip: %w", err)
	}

	defer closer.Close(zipReader)

	for _, zipFile := range zipReader.File {
		if filepath.Base(zipFile.Name) == binaryName {
			if zipFile.UncompressedSize64 > uint64(maxExtractedBinaryBytes) {
				return fmt.Errorf("zip entry %q exceeds extraction limit", zipFile.Name)
			}

			zipEntryReader, err := zipFile.Open()
			if err != nil {
				return fmt.Errorf("opening zip entry: %w", err)
			}

			defer closer.Close(zipEntryReader)

			if err := copyExact(dst, zipEntryReader, int64(zipFile.UncompressedSize64)); err != nil {
				return fmt.Errorf("extracting binary from zip: %w", err)
			}

			return nil
		}
	}

	return fmt.Errorf("binary %q not found in zip archive", binaryName)
}

func extractBinaryFromTarGz(r io.Reader, dst io.Writer, binaryName string) error {
	gzReader, err := gzip.NewReader(r)
	if err != nil {
		return fmt.Errorf("opening gzip stream: %w", err)
	}

	defer closer.Close(gzReader)

	tarReader := tar.NewReader(gzReader)
	for {
		hdr, err := tarReader.Next()
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
			if err := copyExact(dst, tarReader, hdr.Size); err != nil {
				return fmt.Errorf("extracting binary: %w", err)
			}

			return nil
		}
	}

	return fmt.Errorf("binary %q not found in archive", binaryName)
}

// WriteEnvFile writes a .env file to dir/.env.
// envVars contains populated KEY=value pairs; placeholders lists var names to
// include as commented-out stubs (# KEY=) for vars the user skipped.
// The file is created with mode 0600 (owner-readable only).
func WriteEnvFile(dir string, envVars map[string]string, placeholders []string) (string, error) {
	if err := os.MkdirAll(dir, dirPerm); err != nil {
		return "", fmt.Errorf("creating directory: %w", err)
	}

	names := make([]string, 0, len(envVars))
	for k := range envVars {
		names = append(names, k)
	}

	sort.Strings(names)

	stubSet := make(map[string]bool, len(placeholders))
	for _, p := range placeholders {
		if _, set := envVars[p]; !set {
			stubSet[p] = true
		}
	}

	stubs := make([]string, 0, len(stubSet))
	for k := range stubSet {
		stubs = append(stubs, k)
	}

	sort.Strings(stubs)

	var builder strings.Builder

	for _, name := range names {
		builder.WriteString(name)
		builder.WriteString("=")
		builder.WriteString(envVars[name])
		builder.WriteString("\n")
	}

	for _, name := range stubs {
		builder.WriteString("# ")
		builder.WriteString(name)
		builder.WriteString("=\n")
	}

	envPath := filepath.Join(dir, ".env")
	if err := os.WriteFile(envPath, []byte(builder.String()), privateFilePerm); err != nil {
		return "", fmt.Errorf("writing .env file: %w", err)
	}

	return envPath, nil
}

// WriteConfig writes the provided TOML content to configDir/configFileName.
func WriteConfig(configDir, configFileName, content string) (string, error) {
	if err := os.MkdirAll(configDir, dirPerm); err != nil {
		return "", fmt.Errorf("creating config directory: %w", err)
	}

	configPath := filepath.Join(configDir, configFileName)
	if err := os.WriteFile(configPath, []byte(content), readWriteFilePerm); err != nil {
		return "", fmt.Errorf("writing config file: %w", err)
	}

	return configPath, nil
}

// CopyConfig copies an existing config file to destDir/configFileName.
func CopyConfig(srcPath, destDir, configFileName string) error {
	srcPath = filepath.Clean(srcPath)
	destDir = filepath.Clean(destDir)
	configFileName = filepath.Base(configFileName)

	if configFileName == "" || configFileName == "." || configFileName == ".." {
		return fmt.Errorf("invalid config file name %q", configFileName)
	}

	if err := os.MkdirAll(destDir, dirPerm); err != nil {
		return fmt.Errorf("creating directory: %w", err)
	}

	data, err := os.ReadFile(srcPath)
	if err != nil {
		return fmt.Errorf("reading config: %w", err)
	}

	destPath := filepath.Join(destDir, configFileName)
	//nolint:gosec // destPath is constrained to destDir + basename(configFileName).
	if err := os.WriteFile(destPath, data, readWriteFilePerm); err != nil {
		return fmt.Errorf("copying config: %w", err)
	}

	return nil
}

// DefaultInstallDir returns a sensible default install directory for the current OS.
func DefaultInstallDir() string {
	switch runtime.GOOS {
	case windowsOS:
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
	case windowsOS:
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
func platformSuffixes() (string, string) {
	return strings.ToLower(runtime.GOOS), strings.ToLower(runtime.GOARCH)
}

func releaseAPIPath(githubRepo, version string) string {
	if version == "" || version == "dev" {
		return fmt.Sprintf("/repos/%s/releases/latest", githubRepo)
	}

	return fmt.Sprintf("/repos/%s/releases/tags/%s", githubRepo, url.PathEscape(version))
}

func fetchRelease(ctx context.Context, releasePath string) (*githubRelease, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, githubAPIURL, nil)
	if err != nil {
		return nil, fmt.Errorf("building GitHub API request: %w", err)
	}

	req.URL.Path = releasePath

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("contacting GitHub API: %w", err)
	}

	defer closer.Close(resp.Body)

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		if resp.StatusCode == http.StatusNotFound {
			return nil, fmt.Errorf("no releases found (404)\nURL: %s\nBody: %s", req.URL.String(), body)
		}

		return nil, fmt.Errorf("GitHub API returned %d\nURL: %s\nBody: %s", resp.StatusCode, req.URL.String(), body)
	}

	var release githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, fmt.Errorf("decoding GitHub response: %w", err)
	}

	return &release, nil
}

func selectMatchingAsset(release *githubRelease, binaryName string) (*Release, error) {
	osSuffix, archSuffix := platformSuffixes()
	binaryName = strings.ToLower(binaryName)

	for _, asset := range release.Assets {
		name := strings.ToLower(asset.Name)
		if strings.Contains(name, binaryName) && strings.Contains(name, osSuffix) && strings.Contains(name, archSuffix) {
			return &Release{
				Version:     release.TagName,
				DownloadURL: asset.BrowserDownloadURL,
				AssetName:   asset.Name,
			}, nil
		}
	}

	return nil, fmt.Errorf("no asset found for %s/%s in release %s", runtime.GOOS, runtime.GOARCH, release.TagName)
}

func newValidatedGETRequest(ctx context.Context, rawURL string) (*http.Request, error) {
	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("invalid download URL: %w", err)
	}

	if parsedURL.Scheme != "https" || parsedURL.Host == "" {
		return nil, fmt.Errorf("invalid download URL: expected https URL with host")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, httpPlaceholderURL, nil)
	if err != nil {
		return nil, fmt.Errorf("building download request: %w", err)
	}

	req.URL = parsedURL

	return req, nil
}

func copyExact(dst io.Writer, src io.Reader, size int64) error {
	if size < 0 {
		return fmt.Errorf("invalid entry size %d", size)
	}

	if size > maxExtractedBinaryBytes {
		return fmt.Errorf("entry size %d exceeds extraction limit %d", size, maxExtractedBinaryBytes)
	}

	if size == 0 {
		return nil
	}

	if _, err := io.CopyN(dst, src, size); err != nil {
		return fmt.Errorf("copying exact size: %w", err)
	}

	return nil
}
