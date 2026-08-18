package proxy

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestMergeSourceRecordsKeepsFirstSeenAndBestCounts(t *testing.T) {
	now := time.Now().UTC()
	existing := SourceDB{
		Version: 1,
		Sources: []SourceRecord{
			{Type: "repository", ID: "a/b", URL: "https://github.com/a/b", Path: "http.txt", DownloadURL: "https://raw.test/a/b/main/http.txt", CandidatesFound: 100, FirstSeen: "2026-01-01T00:00:00Z", LastSeen: "2026-01-02T00:00:00Z"},
		},
	}
	records := []SourceRecord{
		{Type: "repository", ID: "a/b", URL: "https://github.com/a/b", Path: "http.txt", DownloadURL: "https://raw.test/a/b/main/http.txt", CandidatesFound: 50},
		{Type: "gist", ID: "abc", URL: "https://gist.github.com/abc", Path: "proxies.txt", DownloadURL: "https://gist.test/abc/raw/proxies.txt", CandidatesFound: 7},
	}

	merged := MergeSourceRecords(existing, records, now)

	if len(merged.Sources) != 2 {
		t.Fatalf("expected 2 sources, got %d", len(merged.Sources))
	}

	byURL := make(map[string]SourceRecord)
	for _, record := range merged.Sources {
		byURL[record.DownloadURL] = record
	}

	existingRecord := byURL["https://raw.test/a/b/main/http.txt"]
	if existingRecord.CandidatesFound != 100 {
		t.Fatalf("expected best candidate count kept, got %d", existingRecord.CandidatesFound)
	}
	if existingRecord.FirstSeen != "2026-01-01T00:00:00Z" {
		t.Fatalf("expected first seen preserved, got %s", existingRecord.FirstSeen)
	}
	if existingRecord.LastSeen != now.Format(time.RFC3339) {
		t.Fatalf("expected last seen refreshed, got %s", existingRecord.LastSeen)
	}

	if _, ok := byURL["https://gist.test/abc/raw/proxies.txt"]; !ok {
		t.Fatalf("expected new gist record added")
	}
}

func TestMergeSourceRecordsCapEvictsStale(t *testing.T) {
	now := time.Now().UTC()
	existing := SourceDB{}
	for i := 0; i < sourceDBMaxTotal; i++ {
		existing.Sources = append(existing.Sources, SourceRecord{
			DownloadURL:     "https://raw.test/old/" + string(rune('a'+i%26)) + string(rune('a'+i/26)) + "/main/http.txt",
			CandidatesFound: 1,
			FirstSeen:       "2026-01-01T00:00:00Z",
			LastSeen:        "2026-01-01T00:00:00Z",
		})
	}
	records := []SourceRecord{
		{Type: "repository", ID: "new/repo", DownloadURL: "https://raw.test/new/repo/main/http.txt", CandidatesFound: 5},
	}

	merged := MergeSourceRecords(existing, records, now)
	if len(merged.Sources) != sourceDBMaxTotal {
		t.Fatalf("expected cap %d, got %d", sourceDBMaxTotal, len(merged.Sources))
	}

	found := false
	for _, record := range merged.Sources {
		if record.DownloadURL == "https://raw.test/new/repo/main/http.txt" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected fresh record to survive cap eviction")
	}
}

func TestPublishSourcesWritesDatabaseAndList(t *testing.T) {
	dir := t.TempDir()
	cfg := Config{OutputDir: dir}
	manifest := RunManifest{
		Sources: []SourceRecord{
			{Type: "repository", ID: "a/b", URL: "https://github.com/a/b", Path: "http.txt", DownloadURL: "https://raw.test/a/b/main/http.txt", CandidatesFound: 10},
			{Type: "repository", ID: "a/b", URL: "https://github.com/a/b", Path: "socks.txt", DownloadURL: "https://raw.test/a/b/main/socks.txt", CandidatesFound: 3},
		},
	}

	if err := publishSources(cfg, manifest, time.Now().UTC()); err != nil {
		t.Fatalf("publish sources: %v", err)
	}

	db, err := LoadSourceDB(sourcesDBPath(cfg))
	if err != nil {
		t.Fatalf("load sources db: %v", err)
	}
	if len(db.Sources) != 2 {
		t.Fatalf("expected 2 records, got %d", len(db.Sources))
	}

	data, err := os.ReadFile(filepath.Join(dir, "sources.txt"))
	if err != nil {
		t.Fatalf("read sources.txt: %v", err)
	}
	list := string(data)
	if list == "" || list[len(list)-1] != '\n' {
		t.Fatalf("expected newline-terminated sources list, got %q", list)
	}
	if countLines(list) != 2 {
		t.Fatalf("expected 2 lines in sources.txt, got %d", countLines(list))
	}
}

func countLines(content string) int {
	count := 0
	for _, line := range splitLines(content) {
		if line != "" {
			count++
		}
	}
	return count
}

func splitLines(content string) []string {
	var out []string
	start := 0
	for i := 0; i < len(content); i++ {
		if content[i] == '\n' {
			if i > start && content[i-1] == '\r' {
				out = append(out, content[start:i-1])
			} else {
				out = append(out, content[start:i])
			}
			start = i + 1
		}
	}
	if start < len(content) {
		out = append(out, content[start:])
	}
	return out
}
