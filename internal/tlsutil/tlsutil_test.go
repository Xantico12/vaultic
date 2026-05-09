package tlsutil

import (
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestGenerateAndLoad(t *testing.T) {
    dir := t.TempDir()
    certPath := filepath.Join(dir, "vaultic.cert")
    keyPath := filepath.Join(dir, "vaultic.key")

    cert, err := LoadOrGenerateCert(certPath, keyPath)
    if err != nil {
        t.Fatalf("first LoadOrGenerateCert: %v", err)
    }
    if len(cert.Certificate) == 0 {
        t.Fatal("certificate is empty")
    }

    // Reload — should NOT regenerate.
    info1, _ := os.Stat(certPath)
    cert2, err := LoadOrGenerateCert(certPath, keyPath)
    if err != nil {
        t.Fatalf("second LoadOrGenerateCert: %v", err)
    }
    info2, _ := os.Stat(certPath)
    if info1.ModTime() != info2.ModTime() {
        t.Error("cert was regenerated on second call (should have been loaded)")
    }
    if len(cert2.Certificate) == 0 {
        t.Fatal("reloaded certificate is empty")
    }
}

func TestCertHasExpectedSANs(t *testing.T) {
    dir := t.TempDir()
    certPath := filepath.Join(dir, "vaultic.cert")
    keyPath := filepath.Join(dir, "vaultic.key")

    if _, err := LoadOrGenerateCert(certPath, keyPath); err != nil {
        t.Fatalf("LoadOrGenerateCert: %v", err)
    }

    raw, err := os.ReadFile(certPath)
    if err != nil {
        t.Fatalf("ReadFile: %v", err)
    }
    block, _ := pem.Decode(raw)
    if block == nil {
        t.Fatal("PEM decode returned nil")
    }
    parsed, err := x509.ParseCertificate(block.Bytes)
    if err != nil {
        t.Fatalf("ParseCertificate: %v", err)
    }

    // Should be valid for ~5 years.
    expectedExpiry := time.Now().Add(5 * 365 * 24 * time.Hour)
    if parsed.NotAfter.Before(expectedExpiry.Add(-24 * time.Hour)) {
        t.Errorf("NotAfter = %v, expected ~%v", parsed.NotAfter, expectedExpiry)
    }

    // Should include localhost as a DNS SAN.
    foundLocalhost := false
    for _, name := range parsed.DNSNames {
        if name == "localhost" {
            foundLocalhost = true
        }
    }
    if !foundLocalhost {
        t.Errorf("DNSNames missing 'localhost': %v", parsed.DNSNames)
    }

    // Should include 127.0.0.1 as IP SAN.
    found127 := false
    for _, ip := range parsed.IPAddresses {
        if ip.String() == "127.0.0.1" {
            found127 = true
        }
    }
    if !found127 {
        t.Errorf("IPAddresses missing '127.0.0.1': %v", parsed.IPAddresses)
    }
}

func TestKeyFilePermissions(t *testing.T) {
    dir := t.TempDir()
    certPath := filepath.Join(dir, "vaultic.cert")
    keyPath := filepath.Join(dir, "vaultic.key")

    if _, err := LoadOrGenerateCert(certPath, keyPath); err != nil {
        t.Fatalf("LoadOrGenerateCert: %v", err)
    }

    info, err := os.Stat(keyPath)
    if err != nil {
        t.Fatalf("Stat: %v", err)
    }
    perm := info.Mode().Perm()
    if perm != 0600 {
        t.Errorf("key file permissions = %o, want 0600", perm)
    }
}

func TestFingerprintFormat(t *testing.T) {
    fp := Fingerprint([]byte("dummy data"))

    // Should be 32 colon-separated hex pairs.
    parts := strings.Split(fp, ":")
    if len(parts) != 32 {
        t.Errorf("got %d parts, want 32: %s", len(parts), fp)
    }
    for _, p := range parts {
        if len(p) != 2 {
            t.Errorf("part %q is not 2 hex chars", p)
        }
    }
}

func TestFingerprintDeterministic(t *testing.T) {
    a := Fingerprint([]byte("hello"))
    b := Fingerprint([]byte("hello"))
    if a != b {
        t.Errorf("fingerprint not deterministic: %s vs %s", a, b)
    }
}