package auth

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/db"
	"github.com/google/uuid"
)

const (
	PairingKind    = "pairing"
	DeviceKind     = "device"
	BootstrapTTL   = 15 * time.Minute
	QRGrantTTL     = 60 * time.Second
	DeviceTokenTTL = 365 * 24 * time.Hour
)

var ErrPairingUnavailable = errors.New("pairing grant is invalid, expired, or already used")

type Manager struct {
	DB     *db.DB
	Issuer *TokenIssuer
}

func NewManager(database *db.DB, issuer *TokenIssuer) *Manager {
	return &Manager{DB: database, Issuer: issuer}
}

func randomGrant() (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return "cp1_" + base64.RawURLEncoding.EncodeToString(raw), nil
}

func (m *Manager) CreateBootstrapGrant() (string, int64, error) {
	n, err := m.DB.CountAuthDevices()
	if err != nil {
		return "", 0, err
	}
	if n > 0 {
		return "", 0, nil
	}
	return m.createGrant("bootstrap", "", "desktop", "First device", BootstrapTTL)
}

func (m *Manager) CreateDeviceGrant(creator, platform, name string) (string, int64, error) {
	if strings.TrimSpace(platform) == "" || strings.TrimSpace(name) == "" {
		return "", 0, errors.New("device name and platform are required")
	}
	return m.createGrant("device", creator, platform, name, QRGrantTTL)
}

func (m *Manager) createGrant(kind, creator, platform, name string, ttl time.Duration) (string, int64, error) {
	grant, err := randomGrant()
	if err != nil {
		return "", 0, err
	}
	exp := time.Now().Add(ttl).UnixMilli()
	if err := m.DB.InsertPairingGrant(uuid.NewString(), db.HashAuthSecret(grant), kind, creator, platform, name, exp); err != nil {
		return "", 0, err
	}
	return grant, exp, nil
}

type PairedDevice struct {
	ID, Name, Platform, Token string
	ExpiresAt                 int64
}

func (m *Manager) Pair(grant, name, platform string) (PairedDevice, error) {
	if strings.TrimSpace(grant) == "" {
		return PairedDevice{}, ErrPairingUnavailable
	}
	kind, _, targetPlatform, targetName, err := m.DB.ConsumePairingGrant(db.HashAuthSecret(grant), time.Now().UnixMilli())
	if err != nil {
		return PairedDevice{}, ErrPairingUnavailable
	}
	if platform == "" {
		platform = targetPlatform
	}
	if name == "" {
		name = targetName
	}
	deviceID := uuid.NewString()
	token, exp, err := m.Issuer.MintKind(DeviceKind, deviceID, DeviceTokenTTL)
	if err != nil {
		return PairedDevice{}, err
	}
	claims, err := m.Issuer.Claims(token)
	if err != nil {
		return PairedDevice{}, err
	}
	if kind != "bootstrap" && kind != "device" {
		return PairedDevice{}, ErrPairingUnavailable
	}
	if err := m.DB.CreateAuthDevice(db.AuthDevice{ID: deviceID, Name: name, Platform: platform, TokenID: claims.JTI}, db.HashAuthSecret(token)); err != nil {
		return PairedDevice{}, err
	}
	return PairedDevice{ID: deviceID, Name: name, Platform: platform, Token: token, ExpiresAt: exp}, nil
}

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
	return claims, nil
}

func (m *Manager) RevokeDevice(id string) error {
	if strings.TrimSpace(id) == "" {
		return fmt.Errorf("device ID is required")
	}
	return m.DB.RevokeAuthDevice(id)
}

func (m *Manager) Devices() ([]db.AuthDevice, error) { return m.DB.ListAuthDevices() }
