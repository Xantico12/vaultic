package main

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"fmt"
	"io"

	"golang.org/x/crypto/argon2"
)

const (
	KeyLen   = 32  // 256-bit key for AES-256
    SaltLen  = 16  // 128 bits — standard for Argon2
    NonceLen = 12  // 96 bits — required by AES-GCM

    // Argon2id parameters — tuned for ~250-500ms per derivation
    argonTime    = 3
    argonMemory  = 64 * 1024  // 64 MB
    argonThreads = 4
)

func DeriveKey(password, salt []byte) []byte {
	return argon2.IDKey(password, salt, argonTime, argonMemory, argonThreads, KeyLen)
}

func NewSalt() ([]byte, error) {
	salt := make([]byte, SaltLen)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return nil, err
	}
	return salt, nil
}

func Encrypt(key, plaintext []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonce := make([]byte, NonceLen)
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}

	return aead.Seal(nonce, nonce, plaintext, nil), nil
}

func Decrypt(key, blob []byte) ([]byte, error) {
	if len(blob) < NonceLen {
		return nil, fmt.Errorf("ciphertext too short")
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonce := blob[:NonceLen]
	ciphertext := blob[NonceLen:]

	return aead.Open(nil, nonce, ciphertext, nil)
}