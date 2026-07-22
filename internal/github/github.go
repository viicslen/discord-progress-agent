// Package github fetches the worker's commits for today via a baked-in personal
// access token (single-user, so no App-JWT/installation dance). The formatted
// block is appended to the end-of-day report as corroborating output evidence.
// Empty token/username disables the feature; failures are swallowed with a note.
package github

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"discord-tracker-agent/internal/settings"
)

// TodayCommits returns a markdown commit block, or "" if disabled or on error.
// Wired into the engine as Config.CommitsFn.
func TodayCommits() string {
	if settings.GitHubToken == "" || settings.GitHubUsername == "" {
		return ""
	}
	date := time.Now().UTC().Format("2006-01-02")
	q := fmt.Sprintf("author:%s author-date:%s", settings.GitHubUsername, date)
	for _, org := range splitOrgs(settings.GitHubOrgs) {
		q += " org:" + org
	}

	req, err := http.NewRequest("GET", "https://api.github.com/search/commits", nil)
	if err != nil {
		return ""
	}
	req.Header.Set("Authorization", "token "+settings.GitHubToken)
	req.Header.Set("Accept", "application/vnd.github.cloak-preview+json")
	vals := url.Values{}
	vals.Set("q", q)
	vals.Set("per_page", "100")
	vals.Set("sort", "author-date")
	vals.Set("order", "asc")
	req.URL.RawQuery = vals.Encode()

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "" // offline or API error: report proceeds without commits
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return ""
	}

	var out struct {
		Items []struct {
			Commit struct {
				Message string `json:"message"`
			} `json:"commit"`
			Repository struct {
				FullName string `json:"full_name"`
			} `json:"repository"`
		} `json:"items"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return ""
	}
	if len(out.Items) == 0 {
		return ""
	}

	byRepo := map[string][]string{}
	for _, it := range out.Items {
		msg := strings.SplitN(it.Commit.Message, "\n", 2)[0]
		byRepo[it.Repository.FullName] = append(byRepo[it.Repository.FullName], msg)
	}
	repos := make([]string, 0, len(byRepo))
	for r := range byRepo {
		repos = append(repos, r)
	}
	sort.Strings(repos)

	var b strings.Builder
	for _, r := range repos {
		fmt.Fprintf(&b, "**%s**\n", r)
		for _, m := range byRepo[r] {
			fmt.Fprintf(&b, "• %s\n", m)
		}
	}
	return b.String()
}

func splitOrgs(csv string) []string {
	var out []string
	for _, o := range strings.Split(csv, ",") {
		if o = strings.TrimSpace(o); o != "" {
			out = append(out, o)
		}
	}
	return out
}
