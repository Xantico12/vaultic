package vault

import (
	"errors"
	"path/filepath"
	"testing"
)

// helper — returns a fresh WAL path inside a per-test temp directory
func tempWAL(t *testing.T) string {
    t.Helper()
    return filepath.Join(t.TempDir(), "vaultic.wal")
}

func TestStoreRoundtrip(t *testing.T) {
    walPath := tempWAL(t)
    password := "hunter2"

    // First session: write some data
    s1, err := NewStore(walPath, []byte(password))
    if err != nil {
        t.Fatalf("NewStore (first) failed: %v", err)
    }
    if err := s1.Set("api_key", "sk-abc123"); err != nil {
        t.Fatalf("Set failed: %v", err)
    }
    if err := s1.Set("db_password", "swordfish"); err != nil {
        t.Fatalf("Set failed: %v", err)
    }
    s1.Close()

    // Second session: same password should recover the data
    s2, err := NewStore(walPath, []byte(password))
    if err != nil {
        t.Fatalf("NewStore (second) failed: %v", err)
    }
    defer s2.Close()

    val, ok := s2.Get("api_key")
    if !ok || val != "sk-abc123" {
        t.Fatalf("Get(api_key) = (%q, %v), want (sk-abc123, true)", val, ok)
    }

    val, ok = s2.Get("db_password")
    if !ok || val != "swordfish" {
        t.Fatalf("Get(db_password) = (%q, %v), want (swordfish, true)", val, ok)
    }
}

func TestStoreWrongPassword(t *testing.T) {
    walPath := tempWAL(t)

    s1, err := NewStore(walPath, []byte("correct-password"))
    if err != nil {
        t.Fatalf("NewStore (first) failed: %v", err)
    }
    s1.Set("secret", "value")
    s1.Close()

    _, err = NewStore(walPath, []byte("wrong-password"))
    if !errors.Is(err, ErrInvalidPassword) {
        t.Fatalf("NewStore with wrong password = %v, want ErrInvalidPassword", err)
    }
}

func TestStoreDeleteSurvivesRestart(t *testing.T) {
    walPath := tempWAL(t)
    password := "p"

    s1, _ := NewStore(walPath, []byte(password))
    s1.Set("foo", "bar")
    s1.Set("baz", "qux")
    s1.Delete("foo")
    s1.Close()

    s2, err := NewStore(walPath, []byte(password))
    if err != nil {
        t.Fatalf("NewStore failed: %v", err)
    }
    defer s2.Close()

    if _, ok := s2.Get("foo"); ok {
        t.Error("deleted key 'foo' came back after restart")
    }
    if val, ok := s2.Get("baz"); !ok || val != "qux" {
        t.Errorf("baz = (%q, %v), want (qux, true)", val, ok)
    }
}

func TestStoreFirstRunCreatesHeader(t *testing.T) {
    walPath := tempWAL(t)

    s, err := NewStore(walPath, []byte("p"))
    if err != nil {
        t.Fatalf("NewStore failed: %v", err)
    }
    s.Close()

    // Reopen — if header wasn't written correctly, this fails
    if _, err := NewStore(walPath, []byte("p")); err != nil {
        t.Fatalf("reopen after first-run init failed: %v", err)
    }
}