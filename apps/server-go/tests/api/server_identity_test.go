// Pairing URL and server identity coverage: the URL round-trips, the
// identity is stable, and a QR grant's scope is enforced at redemption.
package tests

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/auth"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/db"
)

func TestPairingURLRoundTrips(t *testing.T) {
	url := auth.PairingURL("http://192.168.1.10:3000", "server-123", "vw9g-zu89-aphc")
	// The fragment is query-encoded, so its keys come out sorted.
	if url != "http://192.168.1.10:3000/console/pair#code=vw9g-zu89-aphc&server=server-123&v=1" {
		t.Fatalf("url: %s", url)
	}

	parsed, err := auth.ParsePairingURL(url)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.BaseURL != "http://192.168.1.10:3000" {
		t.Fatalf("base url: %q", parsed.BaseURL)
	}
	if parsed.ServerID != "server-123" {
		t.Fatalf("server id: %q", parsed.ServerID)
	}
	if parsed.Code != "vw9g-zu89-aphc" {
		t.Fatalf("code: %q", parsed.Code)
	}
}

// The grant is in the fragment, so it is never sent to the server. A URL
// that carried it in the query string would put a credential in every
// access log along the path.
func TestPairingURLKeepsTheGrantOutOfThePath(t *testing.T) {
	url := auth.PairingURL("http://host:3000", "server-123", "secret-code")
	if !contains(url, "#") {
		t.Fatalf("the code must live in the fragment: %s", url)
	}
	beforeFragment := url[:indexOf(url, "#")]
	if contains(beforeFragment, "secret-code") {
		t.Fatalf("the code leaked outside the fragment: %s", url)
	}
}

func TestParsePairingURLRejectsMalformed(t *testing.T) {
	for name, raw := range map[string]string{
		"empty":            "",
		"no scheme":        "192.168.1.10:3000/console/pair#v=1&server=s&code=c",
		"bad scheme":       "ftp://host/console/pair#v=1&server=s&code=c",
		"no host":          "http:///console/pair#v=1&server=s&code=c",
		"no fragment":      "http://host:3000/console/pair",
		"bad version":      "http://host:3000/console/pair#v=9&server=s&code=c",
		"no server id":     "http://host:3000/console/pair#v=1&code=c",
		"no code":          "http://host:3000/console/pair#v=1&server=s",
		"missing version":  "http://host:3000/console/pair#server=s&code=c",
		"missing all":      "http://host:3000/console/pair#v=1",
		"not a url at all": "://",
	} {
		if _, err := auth.ParsePairingURL(raw); err == nil {
			t.Fatalf("%s must be rejected", name)
		}
	}
}

// The pairing code is what a user types, so it has to be short, and it has
// to survive being typed in a different case or without the dashes.
func TestPairingCodeIsShortAndForgiving(t *testing.T) {
	database, err := db.Open(db.OpenOptions{Path: ":memory:"})
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	issuer := auth.NewTokenIssuerWithSecret([]byte("short-code-secret-32-bytes-lon"))
	manager := auth.NewManager(database, issuer)

	grant, _, err := manager.CreateBootstrapGrant()
	if err != nil {
		t.Fatal(err)
	}
	// Short enough to read out loud: three groups of four.
	if len(grant) > 15 {
		t.Fatalf("a pairing code must be short, got %q", grant)
	}
	if !strings.Contains(grant, "-") {
		t.Fatalf("a pairing code must be grouped for reading, got %q", grant)
	}

	// The same code typed in a different case, without the dashes and with
	// extra spaces redeems the same grant.
	typed := strings.ToUpper(grant)
	typed = strings.ReplaceAll(typed, "-", " ")
	paired, err := manager.Pair(typed, "Phone", "android")
	if err != nil {
		t.Fatalf("a code typed differently must still redeem: %v", err)
	}
	if paired.Platform != "android" {
		t.Fatalf("platform = %q", paired.Platform)
	}
}

func TestServerIdentityIsStable(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CONSOLE_SERVER_STATE_PATH", "")

	first, err := auth.LoadServerIdentityIn(dir)
	if err != nil {
		t.Fatal(err)
	}
	if first.ID == "" {
		t.Fatal("identity has no id")
	}
	if first.Version == "" {
		t.Fatal("identity has no version")
	}

	// A second read must return the same id: a client that trusted an id
	// once has to be able to trust it again.
	second, err := auth.LoadServerIdentityIn(dir)
	if err != nil {
		t.Fatal(err)
	}
	if second.ID != first.ID {
		t.Fatalf("identity changed between reads: %q then %q", first.ID, second.ID)
	}

	// The file is written 0600 in a 0700 directory.
	info, err := os.Stat(filepath.Join(dir, "server-identity.json"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("identity file must be 0600, got %o", info.Mode().Perm())
	}
}

func TestServerIdentityNameIsConfigurable(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CONSOLE_SERVER_STATE_PATH", "")
	t.Setenv("CONSOLE_SERVER_NAME", "Studio Mac")

	identity, err := auth.LoadServerIdentityIn(dir)
	if err != nil {
		t.Fatal(err)
	}
	if identity.Name != "Studio Mac" {
		t.Fatalf("name = %q", identity.Name)
	}
}

// A QR grant is scoped to the platform and name it was minted for. The
// scope has to be enforced when the grant is redeemed, or it is only a
// suggestion — and a grant meant for a phone would pair a laptop.
func TestQRGrantScopeIsEnforcedAtRedemption(t *testing.T) {
	database, err := db.Open(db.OpenOptions{Path: ":memory:"})
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	issuer := auth.NewTokenIssuerWithSecret([]byte("grant-scope-secret-32-bytes-lon"))
	manager := auth.NewManager(database, issuer)

	// Pair a creator so it can mint a grant.
	bootstrap, _, err := manager.CreateBootstrapGrant()
	if err != nil {
		t.Fatal(err)
	}
	creator, err := manager.Pair(bootstrap, "Desktop", "desktop")
	if err != nil {
		t.Fatal(err)
	}

	grant, _, err := manager.CreateDeviceGrant(creator.ID, "android", "My phone")
	if err != nil {
		t.Fatal(err)
	}

	// The redeemer asks for a different platform and name; the grant's
	// scope wins.
	paired, err := manager.Pair(grant, "My laptop", "desktop")
	if err != nil {
		t.Fatal(err)
	}
	if paired.Platform != "android" {
		t.Fatalf("platform = %q, want the grant's scope", paired.Platform)
	}
	if paired.Name != "My phone" {
		t.Fatalf("name = %q, want the grant's scope", paired.Name)
	}
}

// A bootstrap grant is unscoped: it is the first-device grant, and the
// device names itself.
func TestBootstrapGrantTakesTheRedeemersOwnName(t *testing.T) {
	database, err := db.Open(db.OpenOptions{Path: ":memory:"})
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	issuer := auth.NewTokenIssuerWithSecret([]byte("bootstrap-scope-secret-32-bytes"))
	manager := auth.NewManager(database, issuer)
	grant, _, err := manager.CreateBootstrapGrant()
	if err != nil {
		t.Fatal(err)
	}
	paired, err := manager.Pair(grant, "My phone", "android")
	if err != nil {
		t.Fatal(err)
	}
	if paired.Platform != "android" || paired.Name != "My phone" {
		t.Fatalf("scoped bootstrap grant: %+v", paired)
	}
}

// A revoked device must not be able to keep minting credentials for other
// devices: its outstanding grants die with it.
func TestRevokingADeviceKillsItsGrants(t *testing.T) {
	database, err := db.Open(db.OpenOptions{Path: ":memory:"})
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	issuer := auth.NewTokenIssuerWithSecret([]byte("revoke-grants-secret-32-bytes"))
	manager := auth.NewManager(database, issuer)

	bootstrap, _, err := manager.CreateBootstrapGrant()
	if err != nil {
		t.Fatal(err)
	}
	creator, err := manager.Pair(bootstrap, "Desktop", "desktop")
	if err != nil {
		t.Fatal(err)
	}
	grant, _, err := manager.CreateDeviceGrant(creator.ID, "android", "My phone")
	if err != nil {
		t.Fatal(err)
	}

	if err := manager.RevokeDevice(creator.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Pair(grant, "My phone", "android"); err == nil {
		t.Fatal("a grant created by a revoked device must not redeem")
	}
}

// Revoking every other device leaves the caller working.
func TestRevokeOtherDevicesKeepsTheCaller(t *testing.T) {
	database, err := db.Open(db.OpenOptions{Path: ":memory:"})
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	issuer := auth.NewTokenIssuerWithSecret([]byte("revoke-others-secret-32-bytes"))
	manager := auth.NewManager(database, issuer)

	bootstrap, _, err := manager.CreateBootstrapGrant()
	if err != nil {
		t.Fatal(err)
	}
	caller, err := manager.Pair(bootstrap, "Desktop", "desktop")
	if err != nil {
		t.Fatal(err)
	}
	// A second device, minted with an operator grant.
	operator, _, err := auth.CreateGrant(database, "bootstrap", "", "", "", auth.PairingTTL())
	if err != nil {
		t.Fatal(err)
	}
	other, err := manager.Pair(operator, "Phone", "android")
	if err != nil {
		t.Fatal(err)
	}

	count, err := manager.RevokeOtherDevices(caller.ID)
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("revoked %d devices, want 1", count)
	}
	// The caller still authenticates; the other one does not.
	if _, err := manager.Authenticate(caller.Token); err != nil {
		t.Fatalf("the caller must survive: %v", err)
	}
	if _, err := manager.Authenticate(other.Token); err == nil {
		t.Fatal("the other device must be locked out")
	}
}

// The bootstrap grant is a first-run thing: once a device has been paired,
// the server does not fall back into first-run mode just because that
// device was later revoked.
func TestBootstrapGrantIsNotReissuedAfterSetup(t *testing.T) {
	database, err := db.Open(db.OpenOptions{Path: ":memory:"})
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	issuer := auth.NewTokenIssuerWithSecret([]byte("bootstrap-once-secret-32-bytes"))
	manager := auth.NewManager(database, issuer)

	grant, _, err := manager.CreateBootstrapGrant()
	if err != nil {
		t.Fatal(err)
	}
	if grant == "" {
		t.Fatal("a fresh server must have a bootstrap grant")
	}
	paired, err := manager.Pair(grant, "Desktop", "desktop")
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.RevokeDevice(paired.ID); err != nil {
		t.Fatal(err)
	}

	// Revoked the only device: the server has still been set up, so no
	// new bootstrap grant appears on its own.
	again, _, err := manager.CreateBootstrapGrant()
	if err != nil {
		t.Fatal(err)
	}
	if again != "" {
		t.Fatal("a set-up server must not fall back into first-run mode")
	}
}

// The operator path always mints a grant, whether or not the server has
// been paired — that is how a second device gets added.
func TestOperatorGrantIsAlwaysAvailable(t *testing.T) {
	database, err := db.Open(db.OpenOptions{Path: ":memory:"})
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	issuer := auth.NewTokenIssuerWithSecret([]byte("operator-grant-secret-32-bytes"))
	manager := auth.NewManager(database, issuer)

	grant, _, err := manager.CreateOperatorGrant()
	if err != nil {
		t.Fatal(err)
	}
	if grant == "" {
		t.Fatal("an operator grant must always be mintable")
	}
	// And it works.
	if _, err := manager.Pair(grant, "Phone", "android"); err != nil {
		t.Fatalf("operator grant must redeem: %v", err)
	}
}

// A device that authenticates is recorded as seen, so the device list can
// show which machines are actually in use.
func TestAuthenticationRecordsLastSeen(t *testing.T) {
	database, err := db.Open(db.OpenOptions{Path: ":memory:"})
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	issuer := auth.NewTokenIssuerWithSecret([]byte("last-seen-secret-32-bytes-long"))
	manager := auth.NewManager(database, issuer)
	grant, _, err := manager.CreateBootstrapGrant()
	if err != nil {
		t.Fatal(err)
	}
	paired, err := manager.Pair(grant, "Desktop", "desktop")
	if err != nil {
		t.Fatal(err)
	}

	if _, err := manager.Authenticate(paired.Token); err != nil {
		t.Fatal(err)
	}
	devices, err := manager.Devices()
	if err != nil {
		t.Fatal(err)
	}
	if len(devices) != 1 || devices[0].LastSeenAt == nil {
		t.Fatalf("last seen must be recorded: %+v", devices)
	}
	// The throttle must not fight the test: a fresh manager would touch
	// again, but this one has already touched.
	before := *devices[0].LastSeenAt
	time.Sleep(2 * time.Millisecond)
	if _, err := manager.Authenticate(paired.Token); err != nil {
		t.Fatal(err)
	}
	devices, _ = manager.Devices()
	if *devices[0].LastSeenAt < before {
		t.Fatal("last seen must not go backwards")
	}
}
