package proxy

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestSelectCandidatesForShardPartitionsAllCandidates(t *testing.T) {
	candidates := []Candidate{
		{Host: "1.1.1.1", Port: 80},
		{Host: "2.2.2.2", Port: 80},
		{Host: "3.3.3.3", Port: 80},
		{Host: "4.4.4.4", Port: 80},
	}

	seen := make(map[string]struct{})
	for shard := 0; shard < 3; shard++ {
		for _, candidate := range SelectCandidatesForShard(candidates, shard, 3, "2026-08-18") {
			key := candidate.Address()
			if _, ok := seen[key]; ok {
				t.Fatalf("candidate %s assigned to multiple shards", key)
			}
			seen[key] = struct{}{}
		}
	}

	if len(seen) != len(candidates) {
		t.Fatalf("expected all candidates assigned, got %d of %d", len(seen), len(candidates))
	}
}

func TestShardSaltRedistributesCandidates(t *testing.T) {
	candidates := []Candidate{
		{Host: "1.1.1.1", Port: 80},
		{Host: "2.2.2.2", Port: 80},
		{Host: "3.3.3.3", Port: 80},
		{Host: "4.4.4.4", Port: 80},
		{Host: "5.5.5.5", Port: 80},
		{Host: "6.6.6.6", Port: 80},
	}

	first := make(map[string]int)
	for _, candidate := range candidates {
		first[candidate.Address()] = shardForCandidate(candidate, 16, "2026-08-18")
	}

	moved := 0
	for _, candidate := range candidates {
		if shardForCandidate(candidate, 16, "2026-08-19") != first[candidate.Address()] {
			moved++
		}
	}
	if moved == 0 {
		t.Fatalf("expected salt change to redistribute candidates")
	}

	for _, candidate := range candidates {
		if shardForCandidate(candidate, 16, "2026-08-18") != first[candidate.Address()] {
			t.Fatalf("expected stable assignment for same salt")
		}
	}
}

func TestMergeProxyResultsDeduplicates(t *testing.T) {
	results := []ShardResult{
		{ShardIndex: 0, Proxies: []Proxy{{Protocol: ProtocolHTTP, Host: "1.1.1.1", Port: 80, Sources: []string{"a"}}}},
		{ShardIndex: 1, Proxies: []Proxy{{Protocol: ProtocolHTTP, Host: "1.1.1.1", Port: 80, Sources: []string{"b"}}}},
	}

	merged := mergeProxyResults(results)
	if len(merged) != 1 {
		t.Fatalf("expected 1 merged proxy, got %d", len(merged))
	}
	if len(merged[0].Sources) != 2 {
		t.Fatalf("expected merged sources, got %v", merged[0].Sources)
	}
}

func TestFinalizeRunWritesDashboardData(t *testing.T) {
	dir := t.TempDir()
	cfg := Config{OutputDir: dir, GeoIPDisabled: true, HTTPSProbeURL: "", UserAgent: "proxy-pulse-test", EnrichmentTimeout: 0, ValidationTimeout: 0}
	manifest := RunManifest{
		StartedAt:         "2026-03-18T00:00:00Z",
		Status:            "success",
		RequestsMade:      7,
		SourcesScanned:    4,
		FilesScanned:      5,
		CandidatesFound:   9,
		DuplicatesRemoved: 2,
		SourceCounts:      map[string]int{"repository": 4},
	}
	shards := []ShardResult{
		{
			ShardIndex:   0,
			Checked:      3,
			Validated:    2,
			RequestsMade: 5,
			Outcomes: []ShardOutcome{
				{Address: "1.1.1.1:80", OK: true, Protocol: "http"},
				{Address: "2.2.2.2:1080", OK: true, Protocol: "socks5"},
				{Address: "3.3.3.3:8080", OK: false},
			},
			Proxies: []Proxy{
				{Protocol: ProtocolHTTP, Host: "1.1.1.1", Port: 80},
				{Protocol: ProtocolSOCKS5, Host: "2.2.2.2", Port: 1080},
			},
		},
	}

	if err := FinalizeRun(context.Background(), cfg, manifest, shards); err != nil {
		t.Fatalf("finalize run: %v", err)
	}

	dashboard, err := LoadDashboard(filepath.Join(dir, "docs", "data", "dashboard.json"))
	if err != nil {
		t.Fatalf("load dashboard: %v", err)
	}

	if dashboard.Summary.Status != "success" {
		t.Fatalf("expected success status, got %s", dashboard.Summary.Status)
	}
	if dashboard.Summary.CurrentOutputCounts["all"] != 2 {
		t.Fatalf("expected current all count 2, got %d", dashboard.Summary.CurrentOutputCounts["all"])
	}
	if dashboard.Summary.CurrentCountryCounts["unknown"] != 2 {
		t.Fatalf("expected unknown country count 2, got %d", dashboard.Summary.CurrentCountryCounts["unknown"])
	}
	if len(dashboard.History) != 1 {
		t.Fatalf("expected 1 history entry, got %d", len(dashboard.History))
	}
	if dashboard.History[0].Validated != 2 {
		t.Fatalf("expected validated count 2, got %d", dashboard.History[0].Validated)
	}

	if _, err := os.Stat(filepath.Join(dir, "docs", "data", "dashboard.json")); err != nil {
		t.Fatalf("expected dashboard file to exist: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "docs", "data", "proxies.json")); err != nil {
		t.Fatalf("expected proxies dataset to exist: %v", err)
	}

	kg, err := LoadKnownGood(filepath.Join(dir, "data", "known-good.json"))
	if err != nil {
		t.Fatalf("load known-good: %v", err)
	}
	if len(kg.Entries) != 2 {
		t.Fatalf("expected 2 known-good entries, got %d", len(kg.Entries))
	}
	entry := kg.Entries["http://1.1.1.1:80"]
	if entry.Results != "1" || entry.LastPass == "" {
		t.Fatalf("expected new known-good entry with success streak, got %+v", entry)
	}

	if _, err := os.Stat(filepath.Join(dir, "data", "sources.json")); err != nil {
		t.Fatalf("expected sources database to exist: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "sources.txt")); err != nil {
		t.Fatalf("expected sources list to exist: %v", err)
	}
}
