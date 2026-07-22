// Package discord posts to a Discord webhook. Two shapes: a JSON embed (updates,
// system events, the end-of-day report) and a multipart upload (screenshots).
// Shapes mirror the bot (report.go / handlers.go embeds).
package discord

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"time"
)

type Field struct {
	Name   string `json:"name"`
	Value  string `json:"value"`
	Inline bool   `json:"inline"`
}

type Footer struct {
	Text string `json:"text"`
}

type Image struct {
	URL string `json:"url"`
}

type Embed struct {
	Title       string  `json:"title,omitempty"`
	Description string  `json:"description,omitempty"`
	Color       int     `json:"color,omitempty"`
	Fields      []Field `json:"fields,omitempty"`
	Footer      *Footer `json:"footer,omitempty"`
	Image       *Image  `json:"image,omitempty"`
	Timestamp   string  `json:"timestamp,omitempty"`
}

type payload struct {
	Username string  `json:"username,omitempty"`
	Embeds   []Embed `json:"embeds"`
}

const username = "Session Agent"

// SendEmbed posts a single embed as JSON. Returns an error on transport failure
// or a non-2xx response so the caller keeps the item queued.
func SendEmbed(url string, e Embed) error {
	body, err := json.Marshal(payload{Username: username, Embeds: []Embed{e}})
	if err != nil {
		return err
	}
	return do(url, "application/json", bytes.NewReader(body))
}

// SendImage uploads a PNG with an embed that references it via attachment://.
func SendImage(url, filename, imagePath string, e Embed) error {
	f, err := os.Open(imagePath)
	if err != nil {
		return err
	}
	defer f.Close()

	e.Image = &Image{URL: "attachment://" + filename}
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)

	pj, err := json.Marshal(payload{Username: username, Embeds: []Embed{e}})
	if err != nil {
		return err
	}
	if err := w.WriteField("payload_json", string(pj)); err != nil {
		return err
	}
	part, err := w.CreateFormFile("files[0]", filename)
	if err != nil {
		return err
	}
	if _, err := io.Copy(part, f); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	return do(url, w.FormDataContentType(), &buf)
}

func do(url, contentType string, body io.Reader) error {
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Post(url, contentType, body)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("discord webhook: status %d", resp.StatusCode)
	}
	return nil
}

// RFC3339Now is a tiny helper so callers stamp embeds consistently.
func RFC3339Now() string { return time.Now().Format(time.RFC3339) }
