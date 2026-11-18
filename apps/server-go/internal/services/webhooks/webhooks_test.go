package webhooks

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/db"
)

func openTestStore(t *testing.T) *Service {
	t.Helper()
	dir := t.TempDir()
	database, err := db.Open(db.OpenOptions{Path: filepath.Join(dir, "global.db"), StorageDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(database.Close)
	return New(database.Global())
}

func signSHA256(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

func TestCreateMatchAndOnceLifecycle(t *testing.T) {
	store := openTestStore(t)
	now := time.Unix(1_700_000_000, 0)

	sub, err := store.Create("s1", "github", "fix CI", "my-secret", map[string]any{
		"action":               "completed",
		"check_suite.head_sha": "abc123",
		"conclusion":           []any{"failure", "timed_out"},
	}, true, now.Add(time.Hour).Unix(), now)
	if err != nil {
		t.Fatal(err)
	}

	body := []byte(`{"action":"completed","conclusion":"failure","check_suite":{"head_sha":"abc123"}}`)
	sig := signSHA256("my-secret", body)

	// Wrong secret fails with ErrUnauthorized when targeting sub.ID.
	if _, err := store.Match("github", sub.ID, map[string]string{"X-Hub-Signature-256": "sha256=0000"}, "", body, now); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("expected ErrUnauthorized, got %v", err)
	}

	// Non-matching payload returns empty slice.
	otherBody := []byte(`{"action":"completed","conclusion":"success","check_suite":{"head_sha":"abc123"}}`)
	matched, err := store.Match("github", sub.ID, map[string]string{"X-Hub-Signature-256": signSHA256("my-secret", otherBody)}, "", otherBody, now)
	if err != nil || len(matched) != 0 {
		t.Fatalf("expected no match, got %v, err=%v", matched, err)
	}

	// Matching payload + valid HMAC matches.
	matched, err = store.Match("github", sub.ID, map[string]string{"X-Hub-Signature-256": sig}, "", body, now)
	if err != nil || len(matched) != 1 || matched[0].ID != sub.ID {
		t.Fatalf("expected match, got %v, err=%v", matched, err)
	}

	// AfterFire deletes once=true subscription.
	if err := store.AfterFire(sub.ID, nil, now); err != nil {
		t.Fatal(err)
	}
	if got, err := store.Get(sub.ID); err != nil || got != nil {
		t.Fatalf("after fire = %+v, err=%v, want nil", got, err)
	}

	// Deleted subscription returns ErrNotFound.
	if _, err := store.Match("github", sub.ID, map[string]string{"X-Hub-Signature-256": sig}, "", body, now); !errors.Is(err, ErrNotFound) {
		t.Fatalf("deleted sub should return ErrNotFound, got %v", err)
	}
}

func TestTopicBroadcastAndExpiry(t *testing.T) {
	store := openTestStore(t)
	now := time.Unix(1_700_000_000, 0)

	s1, err := store.Create("s1", "ci", "wake s1", "tok1", nil, false, 0, now)
	if err != nil {
		t.Fatal(err)
	}
	s2, err := store.Create("s2", "ci", "wake s2", "tok2", nil, false, now.Add(10*time.Second).Unix(), now)
	if err != nil {
		t.Fatal(err)
	}

	// Topic-level match with tok1 only wakes s1.
	matched, err := store.Match("ci", "", map[string]string{"Authorization": "Bearer tok1"}, "", []byte(`{"ok":true}`), now)
	if err != nil || len(matched) != 1 || matched[0].ID != s1.ID {
		t.Fatalf("matched = %+v, err=%v", matched, err)
	}

	// After expiry, s2 flips to expired.
	matched, err = store.Match("ci", s2.ID, nil, "tok2", []byte(`{}`), now.Add(20*time.Second))
	if err != nil || len(matched) != 0 {
		t.Fatalf("expired sub should not match, got %v, err=%v", matched, err)
	}
	got2, _ := store.Get(s2.ID)
	if got2.Status != StatusExpired {
		t.Fatalf("status = %q, want expired", got2.Status)
	}
}
