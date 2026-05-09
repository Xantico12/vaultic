package auth

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestGenerateUnique(t *testing.T) {
    raw1, rec1, err := Generate(map[string]Permission{"openclaw": PermRead})
    if err != nil {
        t.Fatalf("Generate: %v", err)
    }
    raw2, rec2, err := Generate(map[string]Permission{"openclaw": PermRead})
    if err != nil {
        t.Fatalf("Generate: %v", err)
    }
    if raw1 == raw2 {
        t.Error("Generate produced identical tokens")
    }
    if rec1.Hash == rec2.Hash {
        t.Error("Generate produced identical hashes")
    }
    if !strings.HasPrefix(raw1, TokenPrefix) {
        t.Errorf("token missing prefix: %s", raw1)
    }
}

func TestAuthorizeRoundtrip(t *testing.T) {
    path := filepath.Join(t.TempDir(), "tokens.jsonl")
    reg, err := LoadRegistry(path)
    if err != nil {
        t.Fatalf("LoadRegistry: %v", err)
    }

    raw, rec, _ := Generate(map[string]Permission{
        "openclaw": PermReadWrite,
        "adpulse":  PermRead,
    })
    if err := reg.Add(rec); err != nil {
        t.Fatalf("Add: %v", err)
    }

    // Read on a read-write namespace — allowed
    if _, err := reg.Authorize(raw, "openclaw", PermRead); err != nil {
        t.Errorf("read on RW namespace failed: %v", err)
    }
    // Write on a read-write namespace — allowed
    if _, err := reg.Authorize(raw, "openclaw", PermReadWrite); err != nil {
        t.Errorf("write on RW namespace failed: %v", err)
    }
    // Read on read-only namespace — allowed
    if _, err := reg.Authorize(raw, "adpulse", PermRead); err != nil {
        t.Errorf("read on RO namespace failed: %v", err)
    }
    // Write on read-only namespace — denied
    if _, err := reg.Authorize(raw, "adpulse", PermReadWrite); err != ErrPermissionDenied {
        t.Errorf("write on RO namespace = %v, want ErrPermissionDenied", err)
    }
    // Any access to unknown namespace — denied
    if _, err := reg.Authorize(raw, "other", PermRead); err != ErrPermissionDenied {
        t.Errorf("unknown namespace = %v, want ErrPermissionDenied", err)
    }
}

func TestAuthorizeRejectsInvalidToken(t *testing.T) {
    path := filepath.Join(t.TempDir(), "tokens.jsonl")
    reg, _ := LoadRegistry(path)

    if _, err := reg.Authorize("vt_bogus", "openclaw", PermRead); err != ErrInvalidToken {
        t.Errorf("bogus token = %v, want ErrInvalidToken", err)
    }
    if _, err := reg.Authorize("not-a-vaultic-token", "openclaw", PermRead); err != ErrInvalidToken {
        t.Errorf("non-prefixed token = %v, want ErrInvalidToken", err)
    }
}

func TestPersistenceAcrossLoad(t *testing.T) {
    path := filepath.Join(t.TempDir(), "tokens.jsonl")
    reg1, _ := LoadRegistry(path)

    raw, rec, _ := Generate(map[string]Permission{"foo": PermRead})
    reg1.Add(rec)

    // Reload
    reg2, err := LoadRegistry(path)
    if err != nil {
        t.Fatalf("reload: %v", err)
    }
    if _, err := reg2.Authorize(raw, "foo", PermRead); err != nil {
        t.Errorf("authorize after reload: %v", err)
    }
}

func TestRevokeBlocksFutureAccess(t *testing.T) {
    path := filepath.Join(t.TempDir(), "tokens.jsonl")
    reg, _ := LoadRegistry(path)

    raw, rec, _ := Generate(map[string]Permission{"openclaw": PermReadWrite})
    reg.Add(rec)

    if _, err := reg.Authorize(raw, "openclaw", PermRead); err != nil {
        t.Fatalf("pre-revoke authorize: %v", err)
    }

    if err := reg.Revoke(rec.ID); err != nil {
        t.Fatalf("Revoke: %v", err)
    }

    if _, err := reg.Authorize(raw, "openclaw", PermRead); err != ErrPermissionDenied {
        t.Errorf("post-revoke = %v, want ErrPermissionDenied", err)
    }
}

func TestRevokeSurvivesReload(t *testing.T) {
    path := filepath.Join(t.TempDir(), "tokens.jsonl")
    reg, _ := LoadRegistry(path)

    raw, rec, _ := Generate(map[string]Permission{"openclaw": PermReadWrite})
    reg.Add(rec)
    reg.Revoke(rec.ID)

    reloaded, _ := LoadRegistry(path)
    if _, err := reloaded.Authorize(raw, "openclaw", PermRead); err != ErrPermissionDenied {
        t.Errorf("revocation didn't persist: %v", err)
    }
}

func TestEmptyRegistryFile(t *testing.T) {
    path := filepath.Join(t.TempDir(), "tokens.jsonl")
    reg, err := LoadRegistry(path)
    if err != nil {
        t.Fatalf("LoadRegistry on missing file: %v", err)
    }
    if got := len(reg.List()); got != 0 {
        t.Errorf("empty registry has %d records", got)
    }
}