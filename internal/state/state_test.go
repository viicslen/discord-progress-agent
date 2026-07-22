package state

import (
	"path/filepath"
	"testing"

	"discord-tracker-agent/internal/vault"
)

func init() {
	k := make([]byte, 32)
	for i := range k {
		k[i] = byte(i + 7)
	}
	_ = vault.Init(k)
}

func TestConsentPersists(t *testing.T) {
	p := filepath.Join(t.TempDir(), "state.dat")

	s, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if s.Consent {
		t.Fatal("fresh state should not have consent")
	}

	s.Consent = true
	s.ConsentAt = 123
	if err := s.Save(); err != nil {
		t.Fatal(err)
	}

	s2, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if !s2.Consent {
		t.Fatal("consent did not persist")
	}
}

func TestSeqMonotonicAcrossReload(t *testing.T) {
	p := filepath.Join(t.TempDir(), "state.dat")
	s, _ := Load(p)

	var prev int64
	for i := 0; i < 5; i++ {
		n := s.Take()
		if n <= prev {
			t.Fatalf("seq not increasing: %d after %d", n, prev)
		}
		prev = n
	}
	if err := s.Save(); err != nil {
		t.Fatal(err)
	}

	s2, _ := Load(p)
	n := s2.Take()
	if n <= prev {
		t.Fatalf("seq reset across reload: got %d, want > %d", n, prev)
	}
}
