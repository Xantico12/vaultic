package main

import (
	"bytes"
	"testing"
)

func TestEncryptDecryptRoundtrip(t *testing.T) {
    salt, err := NewSalt()
    if err != nil {
        t.Fatalf("NewSalt failed: %v", err)
    }

    key := DeriveKey([]byte("hunter2"), salt)
    plaintext := []byte("sk-very-secret-api-key")

    ciphertext, err := Encrypt(key, plaintext)
    if err != nil {
        t.Fatalf("Encrypt failed: %v", err)
    }

    if bytes.Equal(ciphertext, plaintext) {
        t.Fatal("ciphertext equals plaintext — encryption did nothing")
    }

    recovered, err := Decrypt(key, ciphertext)
    if err != nil {
        t.Fatalf("Decrypt failed: %v", err)
    }

    if !bytes.Equal(recovered, plaintext) {
        t.Fatalf("roundtrip mismatch: got %q, want %q", recovered, plaintext)
    }
}

func TestDecryptWrongKey(t *testing.T) {
    salt, _ := NewSalt()
    rightKey := DeriveKey([]byte("correct-password"), salt)
    wrongKey := DeriveKey([]byte("wrong-password"), salt)

    ciphertext, err := Encrypt(rightKey, []byte("secret"))
    if err != nil {
        t.Fatalf("Encrypt failed: %v", err)
    }

    if _, err := Decrypt(wrongKey, ciphertext); err == nil {
        t.Fatal("Decrypt with wrong key should have failed but didn't")
    }
}

func TestDecryptTamperedCiphertext(t *testing.T) {
    salt, _ := NewSalt()
    key := DeriveKey([]byte("password"), salt)

    ciphertext, _ := Encrypt(key, []byte("important data"))

    // Flip a single bit in the ciphertext
    ciphertext[len(ciphertext)-1] ^= 0x01

    if _, err := Decrypt(key, ciphertext); err == nil {
        t.Fatal("Decrypt of tampered ciphertext should have failed")
    }
}

func TestNonceUniqueness(t *testing.T) {
    salt, _ := NewSalt()
    key := DeriveKey([]byte("password"), salt)

    a, _ := Encrypt(key, []byte("same plaintext"))
    b, _ := Encrypt(key, []byte("same plaintext"))

    if bytes.Equal(a, b) {
        t.Fatal("two encryptions produced identical ciphertext — nonce isn't random")
    }
}