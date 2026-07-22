// Package queue is the encrypted, persisted outbox. Real-time updates, system
// events, screenshots, and the end-of-day report all flow through here, so
// anything logged while offline is delivered (in order) once the link returns.
package queue

import (
	"encoding/json"
	"errors"
	"os"
	"sync"

	"discord-tracker-agent/internal/discord"
	"discord-tracker-agent/internal/vault"
)

type Item struct {
	Seq       int64          `json:"seq"`
	Timestamp int64          `json:"ts"`
	Kind      string         `json:"kind"`
	Title     string         `json:"title,omitempty"`   // embed title for simple posts
	Content   string         `json:"content,omitempty"` // embed description
	Color     int            `json:"color,omitempty"`
	ImagePath string         `json:"image_path,omitempty"`
	ImageSHA  string         `json:"image_sha,omitempty"`
	Filename  string         `json:"filename,omitempty"`
	Embed     *discord.Embed `json:"embed,omitempty"` // prebuilt (report)
}

type Queue struct {
	mu    sync.Mutex
	Items []Item `json:"items"`
	path  string
}

func Load(path string) (*Queue, error) {
	b, err := vault.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return &Queue{path: path}, nil
	}
	if err != nil {
		return nil, err
	}
	var q Queue
	if err := json.Unmarshal(b, &q); err != nil {
		return nil, err
	}
	q.path = path
	return &q, nil
}

// Add appends an item and persists. Safe for concurrent callers.
func (q *Queue) Add(it Item) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.Items = append(q.Items, it)
	return q.save()
}

// Next returns the oldest queued item.
func (q *Queue) Next() (Item, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.Items) == 0 {
		return Item{}, false
	}
	return q.Items[0], true
}

// Remove drops the item with the given seq and persists.
func (q *Queue) Remove(seq int64) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	for i, it := range q.Items {
		if it.Seq == seq {
			q.Items = append(q.Items[:i], q.Items[i+1:]...)
			break
		}
	}
	return q.save()
}

func (q *Queue) Len() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.Items)
}

// Drain sends queued items oldest-first via send, removing each on success and
// stopping at the first failure so it retries next tick. A sent screenshot's PNG
// is deleted. This is the offline-recovery mechanism: it just picks up where it
// left off after a restart because the queue is persisted.
func (q *Queue) Drain(send func(Item) error) {
	for {
		it, ok := q.Next()
		if !ok {
			return
		}
		if err := send(it); err != nil {
			return
		}
		_ = q.Remove(it.Seq)
		if it.ImagePath != "" {
			_ = os.Remove(it.ImagePath)
		}
	}
}

func (q *Queue) save() error {
	if err := vault.EnsureDir(q.path); err != nil {
		return err
	}
	b, err := json.Marshal(q)
	if err != nil {
		return err
	}
	return vault.WriteFile(q.path, b)
}
