package vault

import (
	"bytes"
	"testing"
)

func testKey() []byte {
	k := make([]byte, 32)
	for i := range k {
		k[i] = byte(i + 1)
	}
	return k
}

func TestSealOpenRoundTrip(t *testing.T) {
	if err := Init(testKey()); err != nil {
		t.Fatal(err)
	}
	plain := []byte("secret-worker-name did some work")
	sealed, err := Seal(plain)
	if err != nil {
		t.Fatal(err)
	}
	got, err := Open(sealed)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, plain) {
		t.Fatalf("round trip mismatch: %q != %q", got, plain)
	}
}

func TestTamperDetected(t *testing.T) {
	if err := Init(testKey()); err != nil {
		t.Fatal(err)
	}
	sealed, _ := Seal([]byte("hello"))
	sealed[len(sealed)-1] ^= 0xFF // flip one byte
	if _, err := Open(sealed); err == nil {
		t.Fatal("expected Open to fail on tampered data")
	}
}

func TestNotPlaintext(t *testing.T) {
	if err := Init(testKey()); err != nil {
		t.Fatal(err)
	}
	secret := []byte("secret-worker-name")
	sealed, _ := Seal(secret)
	if bytes.Contains(sealed, secret) {
		t.Fatal("plaintext leaked into sealed output")
	}
}
