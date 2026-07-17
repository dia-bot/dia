// Package secret encrypts small sensitive values (customer bot tokens, OAuth
// client secrets) for storage at rest with AES-256-GCM.
//
// The key comes from CUSTOM_BOT_ENC_KEY: 32 raw bytes, base64- or hex-encoded.
// Ciphertext is stored as nonce||sealed so a single []byte round-trips through
// a BYTEA column. A Box with no key (feature not configured) fails closed:
// Encrypt/Decrypt return ErrNoKey rather than storing plaintext.
package secret

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strings"
)

// ErrNoKey is returned by a Box that has no key configured.
var ErrNoKey = errors.New("secret: no encryption key configured (set CUSTOM_BOT_ENC_KEY)")

// Box seals and opens secrets with a fixed AES-256-GCM key.
type Box struct {
	aead cipher.AEAD
}

// NewBox builds a Box from an encoded 32-byte key. An empty key yields a Box
// whose Encrypt/Decrypt return ErrNoKey (so the caller can degrade gracefully
// when custom bots aren't configured). The key may be standard/URL base64 or
// hex; it must decode to exactly 32 bytes.
func NewBox(encodedKey string) (*Box, error) {
	encodedKey = strings.TrimSpace(encodedKey)
	if encodedKey == "" {
		return &Box{}, nil
	}
	key, err := decodeKey(encodedKey)
	if err != nil {
		return nil, err
	}
	if len(key) != 32 {
		return nil, fmt.Errorf("secret: key must be 32 bytes (got %d) for AES-256", len(key))
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("secret: new cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("secret: new gcm: %w", err)
	}
	return &Box{aead: aead}, nil
}

// Enabled reports whether a key is configured.
func (b *Box) Enabled() bool { return b != nil && b.aead != nil }

// Encrypt seals plaintext, returning nonce||ciphertext.
func (b *Box) Encrypt(plaintext []byte) ([]byte, error) {
	if !b.Enabled() {
		return nil, ErrNoKey
	}
	nonce := make([]byte, b.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("secret: nonce: %w", err)
	}
	// Seal appends the ciphertext to nonce, so the result is nonce||ciphertext.
	return b.aead.Seal(nonce, nonce, plaintext, nil), nil
}

// EncryptString is Encrypt for a string plaintext.
func (b *Box) EncryptString(s string) ([]byte, error) { return b.Encrypt([]byte(s)) }

// Decrypt opens a nonce||ciphertext blob produced by Encrypt.
func (b *Box) Decrypt(blob []byte) ([]byte, error) {
	if !b.Enabled() {
		return nil, ErrNoKey
	}
	ns := b.aead.NonceSize()
	if len(blob) < ns {
		return nil, errors.New("secret: ciphertext too short")
	}
	nonce, ct := blob[:ns], blob[ns:]
	pt, err := b.aead.Open(nil, nonce, ct, nil)
	if err != nil {
		return nil, fmt.Errorf("secret: decrypt: %w", err)
	}
	return pt, nil
}

// DecryptString is Decrypt returning a string.
func (b *Box) DecryptString(blob []byte) (string, error) {
	pt, err := b.Decrypt(blob)
	return string(pt), err
}

func decodeKey(s string) ([]byte, error) {
	// Try hex first when it looks like hex (64 chars, all hex digits).
	if len(s) == 64 {
		if k, err := hex.DecodeString(s); err == nil {
			return k, nil
		}
	}
	for _, enc := range []*base64.Encoding{base64.StdEncoding, base64.RawStdEncoding, base64.URLEncoding, base64.RawURLEncoding} {
		if k, err := enc.DecodeString(s); err == nil {
			return k, nil
		}
	}
	if k, err := hex.DecodeString(s); err == nil {
		return k, nil
	}
	return nil, errors.New("secret: key is not valid base64 or hex")
}
