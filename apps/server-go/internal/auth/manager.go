package auth

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/db"
	"github.com/google/uuid"
)

const (
	// PairingKind is the grant kind for the first device on a server.
	PairingKind = "bootstrap"
	// OperatorKind is the grant kind the operator asks for explicitly
	// (`console pair`), on a server that has been paired before.
	OperatorKind = "operator"
	// DeviceKind is the QR grant kind: short-lived, scoped to one other
	// device, created by an already-authenticated device.
	DeviceKind = "device"
	// DefaultPairingTTL is how long an operator-issued pairing URL stays
	// valid. Long enough to walk to another machine, short enough that a
	// URL left in a chat log is not a standing invitation.
	DefaultPairingTTL = 15 * time.Minute
	// QRGrantTTL is how long a QR grant stays valid. A phone is scanned in
	// seconds, so this is deliberately tight.
	QRGrantTTL = 60 * time.Second
	// DefaultDeviceTokenTTL is how long a device credential stays valid.
	// Long enough for normal use; revoking the device ends it sooner.
	DefaultDeviceTokenTTL = 365 * 24 * time.Hour
)

// PairingTTL is how long a pairing URL stays valid.
//
// CONSOLE_PAIRING_GRANT_TTL overrides it, as a Go duration ("45m", "2h").
// A value that cannot be read falls back to the default rather than
// failing: a typo in a tuning knob should not stop the server booting.
func PairingTTL() time.Duration {
	return durationFromEnv("CONSOLE_PAIRING_GRANT_TTL", DefaultPairingTTL)
}

// DeviceTokenTTL is how long a device credential stays valid.
//
// CONSOLE_DEVICE_TOKEN_TTL overrides it, as a Go duration ("180d", "2160h").
func DeviceTokenTTL() time.Duration {
	return durationFromEnv("CONSOLE_DEVICE_TOKEN_TTL", DefaultDeviceTokenTTL)
}

// durationFromEnv reads a duration override, falling back when it is unset
// or unreadable.
func durationFromEnv(key string, fallback time.Duration) time.Duration {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	parsed, err := time.ParseDuration(raw)
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}

var ErrPairingUnavailable = errors.New("pairing grant is invalid, expired, or already used")

type Manager struct {
	DB     *db.DB
	Issuer *TokenIssuer
	// Identity is this server's public identity, used in pairing URLs so a
	// client can tell which server it is about to pair with.
	Identity ServerIdentity
	// touchThrottle is how long a device's last_seen_at is allowed to go
	// un-updated. Without it every authenticated request would be a write.
	touchThrottle time.Duration
	// lastTouch remembers the last time each device was touched, so the
	// throttle survives across requests without a second database read.
	lastTouch map[string]time.Time
	touchMu   sync.Mutex
}

func NewManager(database *db.DB, issuer *TokenIssuer) *Manager {
	return &Manager{
		DB:           database,
		Issuer:       issuer,
		touchThrottle: time.Minute,
		lastTouch:    map[string]time.Time{},
	}
}

// randomGrant mints a pairing code.
//
// The code is what a user copies off the server and pastes into a client;
// see pairing_code.go for the format and why it is short.
func randomGrant() (string, error) {
	return randomPairingCode()
}

// CreateBootstrapGrant mints the first-device grant, but only while the
// server has never been paired.
//
// "Never been paired" counts revoked devices too: a server whose only
// device was revoked has been set up, and quietly returning it to
// first-run mode would hand out a fresh credential to whoever asks next.
func (m *Manager) CreateBootstrapGrant() (string, int64, error) {
	n, err := m.DB.CountAllAuthDevices()
	if err != nil {
		return "", 0, err
	}
	if n > 0 {
		return "", 0, nil
	}
	return m.createGrant(PairingKind, "", "", "", PairingTTL())
}

// CreateDeviceGrant mints a short-lived grant that one other device can
// redeem — the QR flow.
//
// The grant is scoped to the requested platform and name. The scope is
// enforced at redemption, not just recorded: a grant minted for a phone
// pairs a phone.
func (m *Manager) CreateDeviceGrant(creator, platform, name string) (string, int64, error) {
	if strings.TrimSpace(platform) == "" || strings.TrimSpace(name) == "" {
		return "", 0, errors.New("device name and platform are required")
	}
	return m.createGrant(DeviceKind, creator, platform, name, QRGrantTTL)
}

// CreateOperatorGrant mints a one-time pairing grant on explicit request,
// whether or not devices are already paired.
//
// This is the "operator explicitly asks for a new bootstrap grant" path.
// The operator is whoever can run the CLI on the server machine, and that
// person can already read the signing secret off disk — so minting them a
// grant grants strictly less power than they already hold. The grant
// itself is one-time and short-lived, so a leaked one buys a single
// pairing and nothing more.
func (m *Manager) CreateOperatorGrant() (string, int64, error) {
	return m.createGrant(OperatorKind, "", "", "", PairingTTL())
}

func (m *Manager) createGrant(kind, creator, platform, name string, ttl time.Duration) (string, int64, error) {
	return CreateGrant(m.DB, kind, creator, platform, name, ttl)
}

// CreateGrant mints a one-time pairing grant and records it (hashed).
//
// It is a plain function rather than a method so the CLI can mint a grant
// without a token issuer: the grant is a random secret, not a signed
// token, so issuing one needs nothing but the database. That is what lets
// `console status` hand the operator a pairing code without the server
// process having to be involved.
//
// The code is stored hashed, and looked up by the hash of its *normalised*
// form, so a user who types it in a different case, with or without the
// dashes, still redeems the same grant.
func CreateGrant(database *db.DB, kind, creator, platform, name string, ttl time.Duration) (string, int64, error) {
	grant, err := randomGrant()
	if err != nil {
		return "", 0, err
	}
	exp := time.Now().Add(ttl).UnixMilli()
	hash := db.HashAuthSecret(normalizePairingCode(grant))
	if err := database.InsertPairingGrant(uuid.NewString(), hash, kind, creator, platform, name, exp); err != nil {
		return "", 0, err
	}
	return grant, exp, nil
}

type PairedDevice struct {
	ID, Name, Platform, Token string
	ExpiresAt                 int64
}

// Pair redeems a one-time grant for this device's own credential.
//
// The grant decides who is pairing. A QR grant carries the platform and
// name it was minted for, and those win: the redeemer cannot talk its way
// into a different scope. A bootstrap grant carries no scope, so the
// redeemer names itself.
func (m *Manager) Pair(grant, name, platform string) (PairedDevice, error) {
	if strings.TrimSpace(grant) == "" {
		return PairedDevice{}, ErrPairingUnavailable
	}
	// The code is looked up by the hash of its normalised form, so the
	// dashes, the spacing and the capitalisation a user typed it with do
	// not matter.
	code := normalizePairingCode(grant)
	if err := validatePairingCode(code); err != nil {
		return PairedDevice{}, ErrPairingUnavailable
	}
	kind, _, targetPlatform, targetName, err := m.DB.ConsumePairingGrant(db.HashAuthSecret(code), time.Now().UnixMilli())
	if err != nil {
		return PairedDevice{}, ErrPairingUnavailable
	}
	if kind != PairingKind && kind != DeviceKind && kind != OperatorKind {
		return PairedDevice{}, ErrPairingUnavailable
	}
	// A scoped grant is binding: the target wins over whatever the
	// redeemer asked for.
	if targetPlatform != "" {
		platform = targetPlatform
	}
	if targetName != "" {
		name = targetName
	}
	if strings.TrimSpace(platform) == "" {
		return PairedDevice{}, ErrPairingUnavailable
	}
	if strings.TrimSpace(name) == "" {
		name = platform
	}
	deviceID := uuid.NewString()
	token, exp, err := m.Issuer.MintKind(DeviceKind, deviceID, DeviceTokenTTL())
	if err != nil {
		return PairedDevice{}, err
	}
	claims, err := m.Issuer.Claims(token)
	if err != nil {
		return PairedDevice{}, err
	}
	if err := m.DB.CreateAuthDevice(db.AuthDevice{ID: deviceID, Name: name, Platform: platform, TokenID: claims.JTI}, db.HashAuthSecret(token)); err != nil {
		return PairedDevice{}, err
	}
	return PairedDevice{ID: deviceID, Name: name, Platform: platform, Token: token, ExpiresAt: exp}, nil
}

// Authenticate validates a device token and reports the device it belongs to.
//
// The database is consulted on every request, not just at token mint: a
// revoked device must lose access immediately, and the only record of a
// revocation lives in the database. The token's own signature proves it was
// issued; the database proves it is still allowed.
func (m *Manager) Authenticate(token string) (Claims, error) {
	claims, err := m.Issuer.Claims(token)
	if err != nil {
		return Claims{}, err
	}
	if claims.Kind != DeviceKind {
		return Claims{}, ErrTokenInvalid
	}
	active, err := m.DB.AuthDeviceActive(claims.Device, claims.JTI)
	if err != nil || !active {
		return Claims{}, ErrTokenRevoked
	}
	m.touch(claims.Device)
	return claims, nil
}

// touch records that a device was seen, at most once per throttle window.
//
// A device that polls every second would otherwise turn every request into
// a database write. The window is short enough that "last seen" still means
// "recently active" in the device list.
func (m *Manager) touch(deviceID string) {
	now := time.Now()
	m.touchMu.Lock()
	last, ok := m.lastTouch[deviceID]
	if ok && now.Sub(last) < m.touchThrottle {
		m.touchMu.Unlock()
		return
	}
	m.lastTouch[deviceID] = now
	m.touchMu.Unlock()
	m.DB.TouchAuthDevice(deviceID)
}

// RevokeDevice locks a device out.
//
// Its live pairing grants die with it: a revoked device must not be able to
// keep minting credentials for other devices.
func (m *Manager) RevokeDevice(id string) error {
	if strings.TrimSpace(id) == "" {
		return fmt.Errorf("device ID is required")
	}
	now := time.Now().UnixMilli()
	if err := m.DB.RevokeAuthDevice(id); err != nil {
		return err
	}
	return m.DB.RevokePairingGrantsForDevice(id, now)
}

// RevokeOtherDevices locks out every device except the one named.
//
// Used by "sign out my other devices": the caller keeps working, everyone
// else is locked out. Returns how many devices were revoked.
func (m *Manager) RevokeOtherDevices(keepID string) (int64, error) {
	if strings.TrimSpace(keepID) == "" {
		return 0, fmt.Errorf("device ID is required")
	}
	n, err := m.DB.RevokeOtherAuthDevices(keepID)
	if err != nil {
		return 0, err
	}
	// The revoked devices' outstanding grants die with them.
	devices, err := m.DB.ListAuthDevices()
	if err != nil {
		return n, err
	}
	now := time.Now().UnixMilli()
	for _, device := range devices {
		if device.ID == keepID {
			continue
		}
		if device.RevokedAt != nil && *device.RevokedAt == now {
			_ = m.DB.RevokePairingGrantsForDevice(device.ID, now)
		}
	}
	return n, nil
}

// RenameDevice sets a device's display name.
func (m *Manager) RenameDevice(id, name string) error {
	if strings.TrimSpace(id) == "" {
		return fmt.Errorf("device ID is required")
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("device name is required")
	}
	return m.DB.RenameAuthDevice(id, name)
}

func (m *Manager) Devices() ([]db.AuthDevice, error) { return m.DB.ListAuthDevices() }

// ServerInfo reports this server's public identity.
//
// The base URL is left empty here: the server knows its own address only
// as a bind host, which is often 0.0.0.0 and useless to a client. The
// pairing URL carries the address the user should actually use.
func (m *Manager) ServerInfo() ServerIdentity { return m.Identity }
