package db

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"time"
)

func HashAuthSecret(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

type AuthDevice struct {
	ID, Name, Platform, TokenID      string
	CreatedAt, LastSeenAt, RevokedAt *int64
}

func (d *DB) CreateAuthDevice(device AuthDevice, tokenHash string) error {
	_, err := d.global.Exec(`INSERT INTO auth_devices (id,name,platform,token_id,token_hash,created_at) VALUES (?,?,?,?,?,?)`, device.ID, device.Name, device.Platform, device.TokenID, tokenHash, valueOrNow(device.CreatedAt))
	return err
}

func valueOrNow(value *int64) int64 {
	if value != nil {
		return *value
	}
	return time.Now().UnixMilli()
}

func (d *DB) AuthDeviceActive(id, tokenID string) (bool, error) {
	var revoked sql.NullInt64
	err := d.global.QueryRow(`SELECT revoked_at FROM auth_devices WHERE id=? AND token_id=?`, id, tokenID).Scan(&revoked)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return !revoked.Valid, nil
}

// TouchAuthDevice records that a device was seen just now.
//
// Throttled on the caller's side (see auth.Manager): this is a write on the
// request path, and a device that polls every second would otherwise turn
// every request into a database write.
func (d *DB) TouchAuthDevice(id string) {
	_, _ = d.global.Exec(`UPDATE auth_devices SET last_seen_at=? WHERE id=? AND revoked_at IS NULL`, time.Now().UnixMilli(), id)
}

// RenameAuthDevice sets a device's display name.
func (d *DB) RenameAuthDevice(id, name string) error {
	_, err := d.global.Exec(`UPDATE auth_devices SET name=? WHERE id=?`, name, id)
	return err
}

// RevokeOtherAuthDevices revokes every device except the one named.
//
// Used by "sign out my other devices": the caller keeps working, everyone
// else is locked out. Returns how many devices were revoked.
func (d *DB) RevokeOtherAuthDevices(keepID string) (int64, error) {
	res, err := d.global.Exec(
		`UPDATE auth_devices SET revoked_at=? WHERE id<>? AND revoked_at IS NULL`,
		time.Now().UnixMilli(), keepID,
	)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (d *DB) ListAuthDevices() ([]AuthDevice, error) {
	rows, err := d.global.Query(`SELECT id,name,platform,token_id,created_at,last_seen_at,revoked_at FROM auth_devices ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AuthDevice
	for rows.Next() {
		var v AuthDevice
		if err := rows.Scan(&v.ID, &v.Name, &v.Platform, &v.TokenID, &v.CreatedAt, &v.LastSeenAt, &v.RevokedAt); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (d *DB) RevokeAuthDevice(id string) error {
	_, err := d.global.Exec(`UPDATE auth_devices SET revoked_at=? WHERE id=? AND revoked_at IS NULL`, time.Now().UnixMilli(), id)
	return err
}

// CountAuthDevices counts the devices that are still active.
func (d *DB) CountAuthDevices() (int, error) {
	var n int
	err := d.global.QueryRow(`SELECT COUNT(*) FROM auth_devices WHERE revoked_at IS NULL`).Scan(&n)
	return n, err
}

// CountAllAuthDevices counts every device ever paired, revoked or not.
//
// This is the "has this server ever been set up" question, which is not the
// same as "does it have live devices": a server whose only device was
// revoked has still been set up, and must not fall back into first-run
// bootstrap mode.
func (d *DB) CountAllAuthDevices() (int, error) {
	var n int
	err := d.global.QueryRow(`SELECT COUNT(*) FROM auth_devices`).Scan(&n)
	return n, err
}

func (d *DB) InsertPairingGrant(id, hash, kind, creator, platform, name string, expires int64) error {
	_, err := d.global.Exec(`INSERT INTO auth_pairing_grants (id,grant_hash,kind,creator_device_id,target_platform,target_name,created_at,expires_at) VALUES (?,?,?,?,?,?,?,?)`, id, hash, kind, creator, platform, name, time.Now().UnixMilli(), expires)
	return err
}

// ConsumePairingGrant atomically redeems a grant for a device.
//
// The grant's target platform and name are authoritative: a QR grant minted
// for "android / My phone" redeems as exactly that, whatever the redeemer
// asks for. Letting the redeemer override them would make the scope a
// suggestion, and a grant meant for a phone would pair a laptop.
//
// The lookup and the consume share one transaction, so two concurrent
// redemptions of the same grant cannot both win.
func (d *DB) ConsumePairingGrant(hash string, now int64) (kind, creator, platform, name string, err error) {
	tx, err := d.global.Begin()
	if err != nil {
		return
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()
	var id string
	err = tx.QueryRow(`SELECT id,kind,COALESCE(creator_device_id,''),target_platform,target_name FROM auth_pairing_grants WHERE grant_hash=? AND expires_at>? AND consumed_at IS NULL AND revoked_at IS NULL`, hash, now).Scan(&id, &kind, &creator, &platform, &name)
	if err != nil {
		return
	}
	res, e := tx.Exec(`UPDATE auth_pairing_grants SET consumed_at=? WHERE id=? AND consumed_at IS NULL`, now, id)
	if e != nil {
		err = e
		return
	}
	n, e := res.RowsAffected()
	if e != nil {
		err = e
		return
	}
	if n != 1 {
		err = fmt.Errorf("pairing grant already consumed")
		return
	}
	err = tx.Commit()
	return
}

// RevokePairingGrantsForDevice invalidates every live grant a device
// created.
//
// A revoked device must not be able to keep minting credentials for other
// devices: its outstanding QR grants die with it.
func (d *DB) RevokePairingGrantsForDevice(creator string, now int64) error {
	_, err := d.global.Exec(
		`UPDATE auth_pairing_grants SET revoked_at=? WHERE creator_device_id=? AND consumed_at IS NULL AND revoked_at IS NULL`,
		now, creator,
	)
	return err
}

// PurgePairingGrants drops grants that can no longer be redeemed.
//
// Expired and already-consumed grants are dead weight: nothing can redeem
// them, and the table would otherwise grow without bound, one row per
// pairing attempt. Revoked grants are kept — they are the audit trail of
// who tried to pair.
func (d *DB) PurgePairingGrants(now int64) {
	_, _ = d.global.Exec(`DELETE FROM auth_pairing_grants WHERE expires_at<? OR consumed_at IS NOT NULL`, now-24*time.Hour.Milliseconds())
}
