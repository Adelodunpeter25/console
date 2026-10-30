// Integration status route coverage: gh detection via a fake binary on
// PATH (installed + version), the missing-binary shape, and a present but
// unparseable `gh --version` degrading to installed-without-version.
package tests

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/routes"
	integrationssvc "github.com/Adelodunpeter25/console/apps/server-go/internal/services/integrations"
)

// newIntegrationsApp builds a Fiber app with only the integration routes.
func newIntegrationsApp() *fiber.App {
	app := fiber.New()
	routes.RegisterIntegrationRoutes(app)
	return app
}

// getIntegrations GETs the endpoint and decodes the body.
func getIntegrations(t *testing.T, app *fiber.App) (int, map[string]any) {
	t.Helper()
	req := httptest.NewRequest("GET", "/api/integrations", nil)
	resp, err := app.Test(req, 10000)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(resp.Body)
	var out map[string]any
	_ = json.Unmarshal(raw, &out)
	return resp.StatusCode, out
}

// fakeGh writes an executable `gh` shim printing script on stdout and
// prepends its dir to PATH for the test.
func fakeGh(t *testing.T, script string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "gh")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+script+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+":"+os.Getenv("PATH"))
}

func ghRow(t *testing.T, out map[string]any) map[string]any {
	t.Helper()
	data, ok := out["data"].(map[string]any)
	if !ok {
		t.Fatalf("missing data envelope: %v", out)
	}
	rows, ok := data["integrations"].([]any)
	if !ok || len(rows) != 1 {
		t.Fatalf("expected exactly the gh row: %v", data)
	}
	row, ok := rows[0].(map[string]any)
	if !ok || row["id"] != "gh" {
		t.Fatalf("expected the gh row: %v", rows)
	}
	return row
}

func TestIntegrationsGhInstalled(t *testing.T) {
	fakeGh(t, `echo "gh version 2.75.0 (2025-11-19)"`)
	code, out := getIntegrations(t, newIntegrationsApp())
	if code != 200 || out["success"] != true {
		t.Fatalf("status=%d body=%v", code, out)
	}
	row := ghRow(t, out)
	if row["installed"] != true {
		t.Fatalf("installed must be true: %v", row)
	}
	if row["version"] != "2.75.0" {
		t.Fatalf("version must parse: %v", row)
	}
	if _, ok := row["binaryPath"].(string); !ok {
		t.Fatalf("binaryPath must be set when found: %v", row)
	}
	if row["installUrl"] == "" || row["installHint"] == "" {
		t.Fatalf("install pointer must always be present: %v", row)
	}
}

func TestIntegrationsGhMissing(t *testing.T) {
	t.Setenv("PATH", t.TempDir()) // empty dir: no gh anywhere
	code, out := getIntegrations(t, newIntegrationsApp())
	if code != 200 || out["success"] != true {
		t.Fatalf("status=%d body=%v", code, out)
	}
	row := ghRow(t, out)
	if row["installed"] != false {
		t.Fatalf("installed must be false: %v", row)
	}
	// protojson drops unset optionals — a missing binary carries neither.
	if _, ok := row["version"]; ok {
		t.Fatalf("version must be absent when missing: %v", row)
	}
	if _, ok := row["binaryPath"]; ok {
		t.Fatalf("binaryPath must be absent when missing: %v", row)
	}
	if row["installUrl"] == "" || row["installHint"] == "" {
		t.Fatalf("install pointer must be present especially when missing: %v", row)
	}
}

func TestIntegrationsGhUnparseableVersion(t *testing.T) {
	fakeGh(t, `echo "totally unexpected output"`)
	_, out := getIntegrations(t, newIntegrationsApp())
	row := ghRow(t, out)
	if row["installed"] != true {
		t.Fatalf("a resolving binary is installed even without a version: %v", row)
	}
	if _, ok := row["version"]; ok {
		t.Fatalf("unparseable version must stay absent: %v", row)
	}
}

func TestIntegrationsGhVersionExitNonZero(t *testing.T) {
	fakeGh(t, `echo "boom" >&2; exit 1`)
	_, out := getIntegrations(t, newIntegrationsApp())
	row := ghRow(t, out)
	if row["installed"] != true {
		t.Fatalf("installed must survive a failing version probe: %v", row)
	}
	if _, ok := row["version"]; ok {
		t.Fatalf("failed probe must leave version absent: %v", row)
	}
}

// releaseServer serves a fake cli/cli release: latest tag, tarball, and
// checksums. corruptChecksum flips the hash so the mismatch path is covered.
func releaseServer(t *testing.T, version, ghScript string, corruptChecksum bool) *httptest.Server {
	t.Helper()
	assetFile, innerPath, err := integrationssvc.GhReleaseAsset(version)
	if err != nil {
		t.Skipf("platform has no gh asset: %v", err)
	}
	var tarball bytes.Buffer
	gz := gzip.NewWriter(&tarball)
	tw := tar.NewWriter(gz)
	body := []byte(ghScript)
	if err := tw.WriteHeader(&tar.Header{Name: innerPath, Mode: 0o755, Size: int64(len(body))}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(body); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(tarball.Bytes())
	hash := hex.EncodeToString(sum[:])
	if corruptChecksum {
		hash = strings.Repeat("0", 64)
	}
	checksums := hash + "  " + assetFile + "\n"
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/cli/cli/releases/latest", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"tag_name":"v`+version+`"}`)
	})
	mux.HandleFunc("/download/", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, assetFile) {
			w.Header().Set("Content-Type", "application/gzip")
			_, _ = w.Write(tarball.Bytes())
			return
		}
		if strings.HasSuffix(r.URL.Path, "gh_"+version+"_checksums.txt") {
			_, _ = io.WriteString(w, checksums)
			return
		}
		http.NotFound(w, r)
	})
	return httptest.NewServer(mux)
}

// withIsolatedBox empties PATH (no system gh leaks in), points HOME at a
// temp dir (so ~/.local/bin lands there), forces the Linux installer
// regardless of the test machine's OS, and points the release bases at srv.
// Returns the fake home.
func withIsolatedBox(t *testing.T, srv *httptest.Server) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("PATH", t.TempDir())
	t.Setenv("HOME", home)
	prevOS := integrationssvc.GhInstallOS
	integrationssvc.GhInstallOS = "linux"
	prevAPI, prevDL := integrationssvc.GhReleaseAPIBase, integrationssvc.GhReleaseDownloadBase
	integrationssvc.GhReleaseAPIBase = srv.URL
	integrationssvc.GhReleaseDownloadBase = srv.URL + "/download"
	t.Cleanup(func() {
		integrationssvc.GhInstallOS = prevOS
		integrationssvc.GhReleaseAPIBase = prevAPI
		integrationssvc.GhReleaseDownloadBase = prevDL
	})
	return home
}

func postInstall(t *testing.T, app *fiber.App) (int, map[string]any) {
	t.Helper()
	req := httptest.NewRequest("POST", "/api/integrations/gh/install", nil)
	resp, err := app.Test(req, 120000)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(resp.Body)
	var out map[string]any
	_ = json.Unmarshal(raw, &out)
	return resp.StatusCode, out
}

func TestIntegrationsGhInstall(t *testing.T) {
	srv := releaseServer(t, "9.9.9", "#!/bin/sh\necho \"gh version 9.9.9 (test)\"\n", false)
	defer srv.Close()
	home := withIsolatedBox(t, srv)

	code, out := postInstall(t, newIntegrationsApp())
	if code != 200 || out["success"] != true {
		t.Fatalf("status=%d body=%v", code, out)
	}
	row := ghRow(t, out)
	if row["installed"] != true || row["version"] != "9.9.9" {
		t.Fatalf("install must report the new binary: %v", row)
	}
	binPath, _ := row["binaryPath"].(string)
	if binPath != filepath.Join(home, ".local", "bin", "gh") {
		t.Fatalf("binary must land in ~/.local/bin: %v", row)
	}
	st, err := os.Stat(binPath)
	if err != nil || st.Mode().Perm()&0o111 == 0 {
		t.Fatalf("installed binary must exist and be executable: %v %v", binPath, err)
	}

	// Detection picks up ~/.local/bin/gh even though PATH lacks the dir.
	_, out = getIntegrations(t, newIntegrationsApp())
	if ghRow(t, out)["installed"] != true {
		t.Fatalf("fresh install must read back as installed: %v", out)
	}
}

func TestIntegrationsGhInstallBrewMissing(t *testing.T) {
	prevOS, prevBrew := integrationssvc.GhInstallOS, integrationssvc.BrewBin
	integrationssvc.GhInstallOS = "darwin"
	integrationssvc.BrewBin = filepath.Join(t.TempDir(), "no-such-brew")
	t.Cleanup(func() {
		integrationssvc.GhInstallOS = prevOS
		integrationssvc.BrewBin = prevBrew
	})
	t.Setenv("PATH", t.TempDir())

	code, out := postInstall(t, newIntegrationsApp())
	if code == 200 || out["success"] != false {
		t.Fatalf("missing brew must fail: status=%d body=%v", code, out)
	}
	msg, _ := out["error"].(string)
	if !strings.Contains(msg, "Homebrew") {
		t.Fatalf("failure must point at Homebrew: %v", out)
	}
}

func TestIntegrationsGhInstallChecksumMismatch(t *testing.T) {
	srv := releaseServer(t, "9.9.9", "#!/bin/sh\necho hi\n", true)
	defer srv.Close()
	withIsolatedBox(t, srv)

	code, out := postInstall(t, newIntegrationsApp())
	if code != 502 || out["success"] != false {
		t.Fatalf("checksum failure must be a 502: status=%d body=%v", code, out)
	}
	if _, ok := out["error"].(string); !ok {
		t.Fatalf("refusal must say why: %v", out)
	}
	_, out = getIntegrations(t, newIntegrationsApp())
	if ghRow(t, out)["installed"] != false {
		t.Fatalf("refused install must stay missing: %v", out)
	}
}

func TestIntegrationsGhInstallSkipsWhenPresent(t *testing.T) {
	// A resolving `gh` short-circuits before any network: point the bases at
	// an unroutable address to prove nothing is fetched.
	fakeGh(t, `echo "gh version 1.2.3 (test)"`)
	integrationssvc.GhReleaseAPIBase = "http://127.0.0.1:1"
	integrationssvc.GhReleaseDownloadBase = "http://127.0.0.1:1"
	t.Cleanup(func() {
		integrationssvc.GhReleaseAPIBase = "https://api.github.com"
		integrationssvc.GhReleaseDownloadBase = "https://github.com/cli/cli/releases/download"
	})

	code, out := postInstall(t, newIntegrationsApp())
	if code != 200 || out["success"] != true {
		t.Fatalf("status=%d body=%v", code, out)
	}
	if ghRow(t, out)["version"] != "1.2.3" {
		t.Fatalf("present binary must be reported untouched: %v", out)
	}
}
