// Package vault is the tamper-resistance core: AES-GCM seal/open plus atomic
// file writes. GCM gives confidentiality (the file is not human-readable) AND
// integrity (any edit fails the auth tag) in one primitive.
//
// ponytail: the key is a build-embedded constant. Known ceiling — a reverse
// engineer can extract it from the binary. The goal is only to stop casual
// "open the file in an editor and change the log", which this fully does.
package vault

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

var gcm cipher.AEAD

// Init must be called once at startup with a 32-byte key.
func Init(key []byte) error {
	if len(key) != 32 {
		return fmt.Errorf("vault: key must be 32 bytes, got %d", len(key))
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return err
	}
	gcm, err = cipher.NewGCM(block)
	return err
}

// Seal returns nonce || AES-GCM(plaintext). A fresh random nonce per call; since
// callers rewrite the whole file each time, nonce reuse is a non-issue.
func Seal(plain []byte) ([]byte, error) {
	if gcm == nil {
		return nil, errors.New("vault: not initialized")
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	return gcm.Seal(nonce, nonce, plain, nil), nil
}

// Open reverses Seal. Any tampering (a single flipped byte) fails the GCM auth
// tag and returns an error, so the caller can treat the file as corrupt.
func Open(sealed []byte) ([]byte, error) {
	if gcm == nil {
		return nil, errors.New("vault: not initialized")
	}
	ns := gcm.NonceSize()
	if len(sealed) < ns {
		return nil, errors.New("vault: sealed data too short")
	}
	nonce, ct := sealed[:ns], sealed[ns:]
	return gcm.Open(nil, nonce, ct, nil)
}

// WriteFile seals plain and writes it atomically (temp + rename) so a crash
// mid-write can never corrupt the existing file.
func WriteFile(path string, plain []byte) error {
	sealed, err := Seal(plain)
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, sealed, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// ReadFile reads and opens a sealed file. Missing file returns os.ErrNotExist.
func ReadFile(path string) ([]byte, error) {
	sealed, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return Open(sealed)
}

// EnsureDir makes the parent directory of path (0700).
func EnsureDir(path string) error {
	return os.MkdirAll(filepath.Dir(path), 0o700)
}
