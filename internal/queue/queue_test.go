package queue

import (
	"os"
	"path/filepath"
	"testing"

	"discord-tracker-agent/internal/vault"
)

func init() {
	k := make([]byte, 32)
	for i := range k {
		k[i] = byte(i + 11)
	}
	_ = vault.Init(k)
}

func TestDrainOrderAndOfflineRetry(t *testing.T) {
	p := filepath.Join(t.TempDir(), "queue.dat")
	q, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	for i := int64(1); i <= 3; i++ {
		if err := q.Add(Item{Seq: i, Content: "u"}); err != nil {
			t.Fatal(err)
		}
	}

	// Offline: send always fails -> nothing drains.
	q.Drain(func(Item) error { return os.ErrClosed })
	if q.Len() != 3 {
		t.Fatalf("offline: expected 3 queued, got %d", q.Len())
	}

	// Online: record seq order as items drain.
	var got []int64
	q.Drain(func(it Item) error {
		got = append(got, it.Seq)
		return nil
	})
	if q.Len() != 0 {
		t.Fatalf("expected empty queue, got %d", q.Len())
	}
	if len(got) != 3 || got[0] != 1 || got[1] != 2 || got[2] != 3 {
		t.Fatalf("drain not in ascending seq order: %v", got)
	}
}

func TestDrainDeletesScreenshot(t *testing.T) {
	dir := t.TempDir()
	png := filepath.Join(dir, "shot.png")
	if err := os.WriteFile(png, []byte("fake"), 0o600); err != nil {
		t.Fatal(err)
	}
	q, _ := Load(filepath.Join(dir, "queue.dat"))
	_ = q.Add(Item{Seq: 1, ImagePath: png})

	q.Drain(func(Item) error { return nil })
	if _, err := os.Stat(png); !os.IsNotExist(err) {
		t.Fatal("screenshot PNG should be deleted after successful send")
	}
}
