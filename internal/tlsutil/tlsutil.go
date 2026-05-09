// Package tlsutil generates and loads self-signed TLS certificates for
// Vaultic's TCP server. On first run, generates a fresh ECDSA P-256
// keypair and writes a self-signed cert + private key to disk.
package tlsutil

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"os"
	"strings"
	"time"
)

// LoadOrGenerateCert returns a usable tls.Certificate, generating a fresh
// self-signed one if certPath or keyPath doesn't exist yet.
func LoadOrGenerateCert(certPath, keyPath string) (tls.Certificate, error) {
    if fileExists(certPath) && fileExists(keyPath) {
        return tls.LoadX509KeyPair(certPath, keyPath)
    }
    return generateAndWrite(certPath, keyPath)
}

// Fingerprint returns a human-readable SHA-256 fingerprint of a DER cert.
// Format: uppercase hex bytes separated by colons (matches openssl, ssh).
func Fingerprint(rawDER []byte) string {
    sum := sha256.Sum256(rawDER)
    parts := make([]string, len(sum))
    for i, b := range sum {
        parts[i] = fmt.Sprintf("%02X", b)
    }
    return strings.Join(parts, ":")
}

// --- internal helpers ---

func fileExists(p string) bool {
    _, err := os.Stat(p)
    return err == nil
}

func generateAndWrite(certPath, keyPath string) (tls.Certificate, error) {
    // 1. Fresh ECDSA P-256 keypair.
    priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
    if err != nil {
        return tls.Certificate{}, fmt.Errorf("generate key: %w", err)
    }

    // 2. Random 128-bit serial number.
    serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
    if err != nil {
        return tls.Certificate{}, fmt.Errorf("generate serial: %w", err)
    }

    // 3. Build the certificate template.
    template := x509.Certificate{
        SerialNumber: serial,
        Subject: pkix.Name{
            Organization: []string{"Vaultic"},
            CommonName:   "vaultic-server",
        },
        NotBefore:             time.Now().Add(-time.Hour), // small skew tolerance
        NotAfter:              time.Now().Add(5 * 365 * 24 * time.Hour),
        KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
        ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
        BasicConstraintsValid: true,
        DNSNames:              []string{"localhost", "vaultic-server"},
        IPAddresses:           []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("::1")},
    }

    // 4. Self-sign: parent cert == this template, parent key == our priv.
    derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
    if err != nil {
        return tls.Certificate{}, fmt.Errorf("create certificate: %w", err)
    }

    // 5. Write cert as PEM (mode 0644 — public-ish).
    if err := writePEM(certPath, "CERTIFICATE", derBytes, 0644); err != nil {
        return tls.Certificate{}, err
    }

    // 6. Marshal private key, write as PEM (mode 0600 — sensitive!).
    keyDER, err := x509.MarshalECPrivateKey(priv)
    if err != nil {
        return tls.Certificate{}, fmt.Errorf("marshal key: %w", err)
    }
    if err := writePEM(keyPath, "EC PRIVATE KEY", keyDER, 0600); err != nil {
        return tls.Certificate{}, err
    }

    // 7. Load it back into a tls.Certificate (combines cert + private key).
    return tls.LoadX509KeyPair(certPath, keyPath)
}

func writePEM(path, blockType string, der []byte, mode os.FileMode) error {
    f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode)
    if err != nil {
        return fmt.Errorf("open %s: %w", path, err)
    }
    defer f.Close()

    if err := pem.Encode(f, &pem.Block{Type: blockType, Bytes: der}); err != nil {
        return fmt.Errorf("encode PEM to %s: %w", path, err)
    }
    return nil
}