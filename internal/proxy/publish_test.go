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
		{Protocol: ProtocolHTTP, Host: "1.1.1.1", Port: 80},
		{Protocol: ProtocolSOCKS5, Host: "2.2.2.2", Port: 1080},
	})
	if err != nil {
		t.Fatalf("publish outputs: %v", err)
	}
	if counts["http"] != 1 || counts["socks5"] != 1 || counts["all"] != 2 {
		t.Fatalf("unexpected counts: %#v", counts)
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
}
