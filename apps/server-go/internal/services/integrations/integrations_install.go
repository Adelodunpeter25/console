// gh CLI install for /api/integrations. When no `gh` resolves on PATH, the
// server installs it the official way for the platform and nothing else:
//
//   - macOS: `brew install gh` (Homebrew owns placement from there)
//   - Linux: the official release tarball from github.com, verified by
//     checksum, unpacked to ~/.local/bin/gh
//
// Deliberately global, never a sidecar dir: the point is that bare `gh`
// works in every agent command, script, and terminal. No sudo anywhere —
// Homebrew and ~/.local/bin are both user-writable. Other platforms answer
// 400 with the download URL so the user can do it by hand.
package integrations

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// GhReleaseAPIBase and GhReleaseDownloadBase are vars, not consts, so tests
// can point them at an httptest server; production code never assigns them.
// Same precedent as github.APIBase.
var GhReleaseAPIBase = "https://api.github.com"
var GhReleaseDownloadBase = "https://github.com/cli/cli/releases/download"

// GhInstallOS and BrewBin are overridable in tests: the former to exercise
// another platform's installer than the test machine's, the latter to fake a
// missing Homebrew. Production code never assigns them.
var GhInstallOS = runtime.GOOS
var BrewBin = "brew"

// ErrGhPlatformUnsupported marks platforms with no install path here.
// Routes map it to 400 (fix by hand) rather than 502 (retry later) via
// errors.Is, so the wording can change without flipping the code.
var ErrGhPlatformUnsupported = errors.New("automatic gh install is not supported on this platform")

// UserBinDir is the standard user binary dir (~/.local/bin). The Linux
// installer targets it, and spawned commands get it on PATH when the daemon
// environment lacks it (see EnsureUserBinDir) — filling a standard gap, not
// shadowing the system: anything already on PATH keeps winning.
func UserBinDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "bin")
}

// EnsureUserBinDir appends the user bin dir to PATH when no entry carries
// it, so a gh installed there (by the installer or by hand) resolves in
// spawned commands even when the daemon runs with a thin PATH. Untouched
// otherwise — and appended, never prepended, so system binaries win.
func EnsureUserBinDir(env []string) []string {
	dir := UserBinDir()
	for _, kv := range env {
		if key, rest, _ := strings.Cut(kv, "="); key == "PATH" {
			for _, entry := range strings.Split(rest, string(os.PathListSeparator)) {
				if entry == dir {
					return env
				}
			}
			out := make([]string, 0, len(env))
			for _, kv := range env {
				if k, _, _ := strings.Cut(kv, "="); k == "PATH" {
					out = append(out, "PATH="+rest+string(os.PathListSeparator)+dir)
				} else {
					out = append(out, kv)
				}
			}
			return out
		}
	}
	return append(env, "PATH="+dir)
}

// GhReleaseAsset maps the server's platform to the official release asset
// file and the binary's path inside the tarball. Exported so the install
// button can show what will be fetched, and so tests can build matching
// fixtures without duplicating the mapping.
func GhReleaseAsset(version string) (assetFile, innerPath string, err error) {
	var osName string
	switch runtime.GOOS {
	case "darwin":
		osName = "macOS"
	case "linux":
		osName = "linux"
	default:
		return "", "", fmt.Errorf("%w (%s) — download it from https://cli.github.com", ErrGhPlatformUnsupported, runtime.GOOS)
	}
	var arch string
	switch runtime.GOARCH {
	case "amd64":
		arch = "amd64"
	case "arm64":
		arch = "arm64"
	default:
		return "", "", fmt.Errorf("%w (%s/%s) — download it from https://cli.github.com", ErrGhPlatformUnsupported, runtime.GOOS, runtime.GOARCH)
	}
	assetFile = fmt.Sprintf("gh_%s_%s_%s.tar.gz", version, osName, arch)
	innerPath = fmt.Sprintf("gh_%s_%s_%s/bin/gh", version, osName, arch)
	return assetFile, innerPath, nil
}

// InstallGhCLI installs gh the official way for the platform. If a `gh`
// already resolves on PATH it returns current status without doing anything:
// install is for the missing case, and the button only shows then.
func InstallGhCLI(ctx context.Context) (IntegrationInfo, error) {
	if _, err := exec.LookPath("gh"); err == nil {
		return CheckGhCLI(), nil
	}
	switch GhInstallOS {
	case "darwin":
		return installGhBrew(ctx)
	case "linux":
		return installGhLinux(ctx)
	default:
		return IntegrationInfo{}, fmt.Errorf("%w (%s) — download it from https://cli.github.com", ErrGhPlatformUnsupported, GhInstallOS)
	}
}

// installGhBrew runs `brew install gh`. Homebrew owns placement; a resolving
// gh afterwards is the only success signal. A missing brew is a fix-by-hand
// error, not a retry: the message says so.
func installGhBrew(ctx context.Context) (IntegrationInfo, error) {
	if _, err := exec.LookPath(BrewBin); err != nil {
		return IntegrationInfo{}, fmt.Errorf("Homebrew is required to install gh automatically — install it from https://brew.sh, then retry")
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	out, err := exec.CommandContext(ctx, BrewBin, "install", "gh").CombinedOutput()
	if err != nil {
		return IntegrationInfo{}, fmt.Errorf("brew install gh failed: %s", firstLines(string(out), 5))
	}
	return CheckGhCLI(), nil
}

// installGhLinux fetches the latest official tarball, verifies its checksum,
// and unpacks the binary to ~/.local/bin/gh.
func installGhLinux(ctx context.Context) (IntegrationInfo, error) {
	tag, err := ghLatestTag(ctx)
	if err != nil {
		return IntegrationInfo{}, err
	}
	version := strings.TrimPrefix(tag, "v")
	assetFile, innerPath, err := GhReleaseAsset(version)
	if err != nil {
		return IntegrationInfo{}, err
	}

	client := &http.Client{Timeout: 5 * time.Minute}
	tmpDir, err := os.MkdirTemp("", "gh-install")
	if err != nil {
		return IntegrationInfo{}, err
	}
	defer os.RemoveAll(tmpDir)

	archivePath := filepath.Join(tmpDir, assetFile)
	if err := downloadFile(ctx, client, GhReleaseDownloadBase+"/"+tag+"/"+assetFile, archivePath); err != nil {
		return IntegrationInfo{}, err
	}
	checksumsURL := GhReleaseDownloadBase + "/" + tag + "/gh_" + version + "_checksums.txt"
	if err := verifyChecksum(ctx, client, checksumsURL, archivePath, assetFile); err != nil {
		return IntegrationInfo{}, err
	}
	dest := filepath.Join(UserBinDir(), "gh")
	if err := extractGhBinary(archivePath, innerPath, dest); err != nil {
		return IntegrationInfo{}, err
	}
	return CheckGhCLI(), nil
}

// ghLatestTag reads the latest cli/cli release tag from the GitHub API.
func ghLatestTag(ctx context.Context) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, GhReleaseAPIBase+"/repos/cli/cli/releases/latest", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "console-agent")
	resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
	if err != nil {
		return "", fmt.Errorf("could not reach github.com to find the latest gh release: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("could not find the latest gh release (github.com answered %d)", resp.StatusCode)
	}
	var parsed struct {
		TagName string `json:"tag_name"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&parsed); err != nil {
		return "", fmt.Errorf("could not read the latest gh release: %w", err)
	}
	if strings.TrimSpace(parsed.TagName) == "" {
		return "", fmt.Errorf("github.com did not return a gh release version")
	}
	return strings.TrimSpace(parsed.TagName), nil
}

// downloadFile fetches url to dest via temp-file-and-rename, so a failed
// download can never leave a partial file at the destination.
func downloadFile(ctx context.Context, client *http.Client, url, dest string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "console-agent")
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("download failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed (%d): %s", resp.StatusCode, url)
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	tmp := fmt.Sprintf("%s.new-%d", dest, os.Getpid())
	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(out, io.LimitReader(resp.Body, 256<<20))
	closeErr := out.Close()
	if copyErr != nil || closeErr != nil {
		_ = os.Remove(tmp)
		if copyErr != nil {
			return fmt.Errorf("download failed: %w", copyErr)
		}
		return fmt.Errorf("download failed: %w", closeErr)
	}
	return os.Rename(tmp, dest)
}

// verifyChecksum checks the archive against the release's checksums.txt. A
// mismatch refuses the install — installing an unverified binary fetched at
// runtime is exactly the risk this path must not take.
func verifyChecksum(ctx context.Context, client *http.Client, checksumsURL, archivePath, assetFile string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, checksumsURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "console-agent")
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("could not fetch gh checksums: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("could not fetch gh checksums (%d)", resp.StatusCode)
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return fmt.Errorf("could not read gh checksums: %w", err)
	}
	var want string
	for _, line := range strings.Split(string(raw), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[1] == assetFile {
			want = fields[0]
			break
		}
	}
	if want == "" {
		return fmt.Errorf("gh checksums did not list %s", assetFile)
	}
	raw, err = os.ReadFile(archivePath)
	if err != nil {
		return err
	}
	sum := sha256.Sum256(raw)
	if got := hex.EncodeToString(sum[:]); !strings.EqualFold(got, want) {
		return fmt.Errorf("gh download failed checksum verification — refusing to install")
	}
	return nil
}

// extractGhBinary pulls exactly innerPath out of the tarball and installs it
// at dest with the executable bit. The entry name must equal the expected
// path exactly — no prefix matching, so a malicious archive cannot smuggle
// in a sibling path (zip-slip is impossible when only one exact name
// extracts).
func extractGhBinary(archivePath, innerPath, dest string) error {
	f, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return fmt.Errorf("gh archive is not a valid tarball: %w", err)
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			return fmt.Errorf("gh archive did not contain %s", innerPath)
		}
		if err != nil {
			return fmt.Errorf("could not read gh archive: %w", err)
		}
		if header.Name != innerPath || !header.FileInfo().Mode().IsRegular() {
			continue
		}
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return err
		}
		tmp := fmt.Sprintf("%s.new-%d", dest, os.Getpid())
		out, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(out, io.LimitReader(tr, 64<<20))
		closeErr := out.Close()
		if copyErr != nil || closeErr != nil {
			_ = os.Remove(tmp)
			if copyErr != nil {
				return fmt.Errorf("could not unpack gh: %w", copyErr)
			}
			return fmt.Errorf("could not unpack gh: %w", closeErr)
		}
		return os.Rename(tmp, dest)
	}
}

// firstLines keeps brew failure output readable: the tail of a Homebrew run
// is pages long, and the reason is up top.
func firstLines(s string, n int) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	if len(lines) > n {
		lines = lines[:n]
	}
	return strings.Join(lines, "\n")
}
