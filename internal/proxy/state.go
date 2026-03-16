package proxy

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

type RunManifest struct {
	StartedAt         string         `json:"started_at"`
	DiscoveryFinished string         `json:"discovery_finished_at"`
	Status            string         `json:"status"`
	RequestsMade      int64          `json:"requests_made"`
	SourcesScanned    int            `json:"sources_scanned"`
	FilesScanned      int            `json:"files_scanned"`
	CandidatesFound   int            `json:"candidates_found"`
	CandidateCount    int            `json:"candidate_count"`
	DuplicatesRemoved int            `json:"duplicates_removed"`
	ErrorCount        int            `json:"error_count"`
	SourceCounts      map[string]int `json:"source_counts"`
}

type ShardResult struct {
	ShardIndex   int     `json:"shard_index"`
	ShardTotal   int     `json:"shard_total"`
	Assigned     int     `json:"assigned"`
	Checked      int     `json:"checked"`
	Validated    int     `json:"validated"`
	ErrorCount   int     `json:"error_count"`
	RequestsMade int64   `json:"requests_made"`
	Proxies      []Proxy `json:"proxies"`
}

func SaveJSON(path string, value any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o644)
}

func LoadJSON(path string, value any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, value)
}

func LoadManifest(path string) (RunManifest, error) {
	var manifest RunManifest
	err := LoadJSON(path, &manifest)
	if errors.Is(err, os.ErrNotExist) {
		return manifest, nil
	}
	return manifest, err
}

func LoadCandidates(path string) ([]Candidate, error) {
	var candidates []Candidate
	err := LoadJSON(path, &candidates)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	return candidates, err
}

func LoadShardResults(paths []string) ([]ShardResult, error) {
	results := make([]ShardResult, 0, len(paths))
	for _, path := range paths {
		var result ShardResult
		if err := LoadJSON(path, &result); err != nil {
			return nil, fmt.Errorf("load shard result %s: %w", path, err)
		}
		results = append(results, result)
	}
	sort.Slice(results, func(i, j int) bool {
		return results[i].ShardIndex < results[j].ShardIndex
	})
	return results, nil
}
