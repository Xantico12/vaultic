// Package auth provides token-based authorization for vaultic-server.
//
// Tokens are 256-bit random values, base64url-encoded with a "vt_" prefix.
// They are presented to the user exactly once at creation; only their
// SHA-256 hash is persisted.
//
// The on-disk format is JSON Lines (jsonl) — one TokenRecord per line.
// New records are appended; revocation appends a new record with the
// same ID and empty permissions, which the loader treats as superseding
// earlier ones.
package auth

import (
	"bufio"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"
)

// Permission is the access level granted on a namespace.
type Permission int

const (
    PermNone      Permission = 0
    PermRead      Permission = 1 // GET, LIST
    PermReadWrite Permission = 2 // + SET, DELETE
    PermAdmin     Permission = 3 // + token management (M6 stretch)
)

// TokenPrefix identifies a Vaultic token visually. Only the prefix matters
// for human eyes — the random bytes after it are what authenticates.
const TokenPrefix = "vt_"

// HashPrefix tags the algorithm used; lets us migrate later (e.g. to argon2).
const HashPrefix = "sha256:"

// ErrInvalidToken is returned when no record matches the presented token.
var ErrInvalidToken = errors.New("invalid token")

// ErrPermissionDenied is returned when the token is valid but lacks the
// required permission for the requested namespace.
var ErrPermissionDenied = errors.New("permission denied")

// TokenRecord is the on-disk shape of a token entry.
type TokenRecord struct {
    ID          string                `json:"id"`
    Hash        string                `json:"hash"`
    Permissions map[string]Permission `json:"perms"`
    CreatedAt   time.Time             `json:"created"`
}

// Registry is an in-memory map of valid tokens, kept in sync with a
// JSONL file on disk.
type Registry struct {
    mu      sync.RWMutex
    path    string
    records map[string]TokenRecord // keyed by hash
}

// LoadRegistry reads (or creates) the registry file at path.
// Empty file = empty registry, which is a valid state.
func LoadRegistry(path string) (*Registry, error) {
    r := &Registry{
        path:    path,
        records: make(map[string]TokenRecord),
    }

    f, err := os.Open(path)
    if err != nil {
        if errors.Is(err, os.ErrNotExist) {
            return r, nil // empty registry, file gets created on first Add
        }
        return nil, err
    }
    defer f.Close()

    scanner := bufio.NewScanner(f)
    for scanner.Scan() {
        line := strings.TrimSpace(scanner.Text())
        if line == "" {
            continue
        }
        var rec TokenRecord
        if err := json.Unmarshal([]byte(line), &rec); err != nil {
            return nil, fmt.Errorf("parse token line: %w", err)
        }
        // Later records override earlier ones (revocation = empty perms).
        r.records[rec.Hash] = rec
    }
    return r, scanner.Err()
}

// Generate creates a fresh token. Returns the raw token (show once to
// the user) and a populated TokenRecord (persist with Add).
func Generate(perms map[string]Permission) (rawToken string, record TokenRecord, err error) {
    var b [32]byte
    if _, err = io.ReadFull(rand.Reader, b[:]); err != nil {
        return "", TokenRecord{}, err
    }
    rawToken = TokenPrefix + base64.RawURLEncoding.EncodeToString(b[:])

    record = TokenRecord{
        ID:          shortID(b[:8]),
        Hash:        hashToken(rawToken),
        Permissions: perms,
        CreatedAt:   time.Now().UTC(),
    }
    return rawToken, record, nil
}

// Add appends a record to the registry file and registers it in memory.
func (r *Registry) Add(record TokenRecord) error {
    r.mu.Lock()
    defer r.mu.Unlock()

    f, err := os.OpenFile(r.path, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0600)
    if err != nil {
        return err
    }
    defer f.Close()

    line, err := json.Marshal(record)
    if err != nil {
        return err
    }
    if _, err := f.Write(append(line, '\n')); err != nil {
        return err
    }
    if err := f.Sync(); err != nil {
        return err
    }

    r.records[record.Hash] = record
    return nil
}

// Authorize verifies a presented raw token and checks namespace permission.
func (r *Registry) Authorize(rawToken, namespace string, want Permission) (*TokenRecord, error) {
    if !strings.HasPrefix(rawToken, TokenPrefix) {
        return nil, ErrInvalidToken
    }
    presented := hashToken(rawToken)

    r.mu.RLock()
    defer r.mu.RUnlock()

    // Constant-time hash comparison across all stored tokens.
    var match *TokenRecord
    for hash, rec := range r.records {
        if subtle.ConstantTimeCompare([]byte(presented), []byte(hash)) == 1 {
            match = &rec
        }
    }
    if match == nil {
        return nil, ErrInvalidToken
    }

    have := match.Permissions[namespace]
    if have < want {
        return match, ErrPermissionDenied
    }
    return match, nil
}

// Revoke records a "deny" entry for the given token ID, superseding any
// earlier permissions for it.
func (r *Registry) Revoke(id string) error {
    r.mu.Lock()
    var existing TokenRecord
    var found bool
    for _, rec := range r.records {
        if rec.ID == id {
            existing = rec
            found = true
            break
        }
    }
    r.mu.Unlock()
    if !found {
        return fmt.Errorf("no token with id %q", id)
    }

    revoked := TokenRecord{
        ID:          id,
        Hash:        existing.Hash,
        Permissions: map[string]Permission{},
        CreatedAt:   time.Now().UTC(),
    }
    return r.Add(revoked)
}

// List returns a snapshot of all current records (including revoked ones,
// which have empty perms).
func (r *Registry) List() []TokenRecord {
    r.mu.RLock()
    defer r.mu.RUnlock()
    out := make([]TokenRecord, 0, len(r.records))
    for _, rec := range r.records {
        out = append(out, rec)
    }
    return out
}

// --- internal helpers ---

func hashToken(raw string) string {
    sum := sha256.Sum256([]byte(raw))
    return HashPrefix + hex.EncodeToString(sum[:])
}

// shortID returns an 8-byte prefix as hex for human display.
// Doesn't need to be unique — collision probability at our scale is negligible.
func shortID(b []byte) string {
    return hex.EncodeToString(b)
}