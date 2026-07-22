// Package state is the encrypted session state: consent, the monotonic sequence
// counter, and the single append-only update log (mirroring the bot's `updates`
// table, where every session event is a row). Persisted via vault (AES-GCM).
package state

import (
	"encoding/json"
	"errors"
	"os"

	"discord-tracker-agent/internal/vault"
)

// Update statuses and kinds (kinds mirror the bot's prefixed content strings).
const (
	StatusOnTime = "ontime"
	StatusLate   = "late"
	StatusMissed = "missed"
)

type Update struct {
	Seq       int64  `json:"seq"`
	Timestamp int64  `json:"ts"`
	Content   string `json:"content"`
	Status    string `json:"status"`
	Kind      string `json:"kind"`
}

type State struct {
	Consent    bool     `json:"consent"`
	ConsentAt  int64    `json:"consent_at"`
	SessionID  int64    `json:"session_id"`
	NextSeq    int64    `json:"next_seq"` // anti-replay/anti-delete anchor, inside the sealed blob
	Updates    []Update `json:"updates"`
	OnBreak    bool     `json:"on_break"`
	BreakStart int64    `json:"break_start"`
	WebhookURL string   `json:"webhook_url,omitempty"` // runtime override; empty => compile-time default

	path string
}

// Load reads sealed state from path. A missing file yields a fresh State needing
// consent. A tamper/corrupt error is surfaced to the caller.
func Load(path string) (*State, error) {
	b, err := vault.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return &State{path: path, SessionID: 1, NextSeq: 1}, nil
	}
	if err != nil {
		return nil, err
	}
	var s State
	if err := json.Unmarshal(b, &s); err != nil {
		return nil, err
	}
	s.path = path
	if s.NextSeq == 0 {
		s.NextSeq = 1
	}
	if s.SessionID == 0 {
		s.SessionID = 1
	}
	return &s, nil
}

func (s *State) Save() error {
	if err := vault.EnsureDir(s.path); err != nil {
		return err
	}
	b, err := json.Marshal(s)
	if err != nil {
		return err
	}
	return vault.WriteFile(s.path, b)
}

// Take returns the next sequence number and advances the counter. Caller Saves.
func (s *State) Take() int64 {
	n := s.NextSeq
	s.NextSeq++
	return n
}

// Append adds an update to the log using the next sequence number.
func (s *State) Append(kind, status, content string, ts int64) Update {
	u := Update{Seq: s.Take(), Timestamp: ts, Content: content, Status: status, Kind: kind}
	s.Updates = append(s.Updates, u)
	return u
}
