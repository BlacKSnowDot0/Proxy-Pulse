package proxy

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	sourceDBVersion  = 1
	sourceDBMaxTotal = 2000
)

type SourceRecord struct {
	Type            string `json:"type"`
	ID              string `json:"id"`
	URL             string `json:"url"`
	Path            string `json:"path"`
	DownloadURL     string `json:"download_url"`
	CandidatesFound int    `json:"candidates_found"`
	FirstSeen       string `json:"first_seen,omitempty"`
	LastSeen        string `json:"last_seen,omitempty"`
}

type SourceDB struct {
	Version   int            `json:"version"`
	UpdatedAt string         `json:"updated_at"`
	Sources   []SourceRecord `json:"sources"`
}

func sourcesDBPath(cfg Config) string {
	return filepath.Join(cfg.OutputDir, "data", "sources.json")
}

func sourcesListPath(cfg Config) string {
	return filepath.Join(cfg.OutputDir, "sources.txt")
}

func LoadSourceDB(path string) (SourceDB, error) {
	var db SourceDB
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return SourceDB{Version: sourceDBVersion}, nil
	}
	if err != nil {
		return SourceDB{}, err
	}
	if err := jsonUnmarshalStrict(data, &db); err != nil {
		return SourceDB{}, fmt.Errorf("load sources db: %w", err)
	}
	return db, nil
}

// MergeSourceRecords folds a run's discovery records into the database:
// DownloadURL is the identity, FirstSeen is kept, LastSeen/CandidatesFound
// track the latest/best observation, and the cap evicts stalest first.
func MergeSourceRecords(existing SourceDB, records []SourceRecord, now time.Time) SourceDB {
	nowISO := now.UTC().Format(time.RFC3339)
	byURL := make(map[string]SourceRecord, len(existing.Sources)+len(records))
	order := make([]string, 0, len(existing.Sources)+len(records))

	for _, record := range existing.Sources {
		if record.DownloadURL == "" {
			continue
		}
		if _, ok := byURL[record.DownloadURL]; !ok {
			order = append(order, record.DownloadURL)
		}
		byURL[record.DownloadURL] = record
	}

	for _, record := range records {
		if record.DownloadURL == "" {
			continue
		}
		current, ok := byURL[record.DownloadURL]
		if !ok {
			record.FirstSeen = nowISO
			byURL[record.DownloadURL] = record
			order = append(order, record.DownloadURL)
			continue
		}
		if record.CandidatesFound > current.CandidatesFound {
			current.CandidatesFound = record.CandidatesFound
		}
		current.LastSeen = nowISO
		current.Type = firstNonEmpty(record.Type, current.Type)
		current.ID = firstNonEmpty(record.ID, current.ID)
		current.URL = firstNonEmpty(record.URL, current.URL)
		current.Path = firstNonEmpty(record.Path, current.Path)
		byURL[record.DownloadURL] = current
	}

	merged := make([]SourceRecord, 0, len(order))
	for _, downloadURL := range order {
		record := byURL[downloadURL]
		if record.FirstSeen == "" {
			record.FirstSeen = nowISO
		}
		if record.LastSeen == "" {
			record.LastSeen = record.FirstSeen
		}
		merged = append(merged, record)
	}

	sort.Slice(merged, func(i, j int) bool {
		if merged[i].LastSeen != merged[j].LastSeen {
			return merged[i].LastSeen > merged[j].LastSeen
		}
		if merged[i].CandidatesFound != merged[j].CandidatesFound {
			return merged[i].CandidatesFound > merged[j].CandidatesFound
		}
		return merged[i].DownloadURL < merged[j].DownloadURL
	})

	if len(merged) > sourceDBMaxTotal {
		merged = merged[:sourceDBMaxTotal]
	}

	return SourceDB{
		Version:   sourceDBVersion,
		UpdatedAt: nowISO,
		Sources:   merged,
	}
}

func publishSources(cfg Config, manifest RunManifest, now time.Time) error {
	existing, err := LoadSourceDB(sourcesDBPath(cfg))
	if err != nil {
		return err
	}

	merged := MergeSourceRecords(existing, manifest.Sources, now)
	if err := SaveJSON(sourcesDBPath(cfg), merged); err != nil {
		return err
	}

	seen := make(map[string]struct{}, len(merged.Sources))
	lines := make([]string, 0, len(merged.Sources))
	for _, record := range merged.Sources {
		if _, ok := seen[record.DownloadURL]; ok {
			continue
		}
		seen[record.DownloadURL] = struct{}{}
		lines = append(lines, record.DownloadURL)
	}
	sort.Strings(lines)

	content := strings.Join(lines, "\n")
	if content != "" {
		content += "\n"
	}
	return os.WriteFile(sourcesListPath(cfg), []byte(content), 0o644)
}
