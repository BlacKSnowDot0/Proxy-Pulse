package proxy

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPublishOutputsAndReadme(t *testing.T) {
	dir := t.TempDir()
	cfg := Config{OutputDir: dir}

	counts, err := PublishOutputs(cfg, []Proxy{
		{Protocol: ProtocolHTTP, Host: "1.1.1.1", Port: 80, ExitIP: "8.8.8.8", CountryCode: "US", CountryName: "United States", Anonymity: AnonymityElite},
		{Protocol: ProtocolSOCKS5, Host: "2.2.2.2", Port: 1080, ExitIP: "1.1.1.1", CountryCode: "AU", CountryName: "Australia", Anonymity: AnonymityUnknown},
	})
	if err != nil {
		t.Fatalf("publish outputs: %v", err)
	}
	if counts["http"] != 1 || counts["socks5"] != 1 || counts["all"] != 2 {
		t.Fatalf("unexpected counts: %#v", counts)
	}
	if _, err := os.Stat(filepath.Join(dir, "docs", "data", "proxies.json")); err != nil {
		t.Fatalf("expected proxies dataset to exist: %v", err)
	}

	stats := StatsDB{
		RunsTotal:           1,
		RequestsTotal:       8,
		ProxiesCheckedTotal: 2,
		ValidatedTotal:      2,
		LastSuccessAt:       "2026-03-16T00:00:00Z",
		LastRun: LastRun{
			FinishedAt:   "2026-03-16T00:00:00Z",
			Status:       "success",
			OutputCounts: counts,
		},
	}
	if err := WriteReadme(dir, stats); err != nil {
		t.Fatalf("write readme: %v", err)
	}

	readmePath := filepath.Join(dir, "README.md")
	data, err := os.ReadFile(readmePath)
	if err != nil {
		t.Fatalf("read readme: %v", err)
	}
	if !strings.Contains(string(data), "Proxy Pulse") {
		t.Fatalf("readme missing project title")
	}
	if !strings.Contains(string(data), "https://blacksnowdot0.github.io/Proxy-Pulse/") {
		t.Fatalf("readme missing dashboard link")
	}
	if !strings.Contains(string(data), "docs/data/proxies.json") {
		t.Fatalf("readme missing proxy dataset link")
	}
}
