package proxy

import (
	"fmt"
	"testing"
)

func TestBuildDashboardAppendsFirstHistoryEntry(t *testing.T) {
	stats := StatsDB{
		RunsTotal:           1,
		RequestsTotal:       9,
		ProxiesCheckedTotal: 5,
		ValidatedTotal:      2,
		LastSuccessAt:       "2026-03-18T00:00:10Z",
		LastRun: LastRun{
			FinishedAt:   "2026-03-18T00:00:10Z",
			Status:       "success",
			RequestsMade: 9,
			Validated:    2,
			OutputCounts: map[string]int{"http": 1, "socks4": 0, "socks5": 1, "all": 2},
			SourceCounts: map[string]int{"repository": 3},
		},
	}

	dashboard := BuildDashboard(DashboardData{}, stats, nil)

	if len(dashboard.History) != 1 {
		t.Fatalf("expected 1 history entry, got %d", len(dashboard.History))
	}
	if dashboard.History[0].FinishedAt != stats.LastRun.FinishedAt {
		t.Fatalf("expected history timestamp %s, got %s", stats.LastRun.FinishedAt, dashboard.History[0].FinishedAt)
	}
}

func TestBuildDashboardReplacesDuplicateTimestampEntry(t *testing.T) {
	stats := StatsDB{
		LastRun: LastRun{
			FinishedAt:   "2026-03-18T00:00:10Z",
			Status:       "success_with_errors",
			Validated:    3,
			OutputCounts: map[string]int{"all": 3},
		},
	}
	existing := DashboardData{
		History: []DashboardHistoryEntry{
			{FinishedAt: "2026-03-18T00:00:10Z", Status: "success", Validated: 1, OutputCounts: map[string]int{"all": 1}},
		},
	}

	dashboard := BuildDashboard(existing, stats, nil)

	if len(dashboard.History) != 1 {
		t.Fatalf("expected 1 history entry, got %d", len(dashboard.History))
	}
	if dashboard.History[0].Status != "success_with_errors" {
		t.Fatalf("expected replaced status, got %s", dashboard.History[0].Status)
	}
	if dashboard.History[0].Validated != 3 {
		t.Fatalf("expected replaced validated count, got %d", dashboard.History[0].Validated)
	}
}

func TestBuildDashboardTrimsHistoryToLimit(t *testing.T) {
	history := make([]DashboardHistoryEntry, 0, dashboardHistoryLimit)
	for i := 0; i < dashboardHistoryLimit; i++ {
		history = append(history, DashboardHistoryEntry{
			FinishedAt: fmt.Sprintf("2026-03-17T%02d:%02d:00Z", i/60, i%60),
		})
	}

	stats := StatsDB{
		LastRun: LastRun{
			FinishedAt: "2026-03-19T00:00:00Z",
			OutputCounts: map[string]int{
				"all": 1,
			},
		},
	}

	dashboard := BuildDashboard(DashboardData{History: history}, stats, nil)

	if len(dashboard.History) != dashboardHistoryLimit {
		t.Fatalf("expected history length %d, got %d", dashboardHistoryLimit, len(dashboard.History))
	}
	if dashboard.History[0].FinishedAt == "2026-03-17T00:00:00Z" {
		t.Fatalf("expected oldest entry to be trimmed")
	}
	if dashboard.History[len(dashboard.History)-1].FinishedAt != "2026-03-19T00:00:00Z" {
		t.Fatalf("expected newest entry at end, got %s", dashboard.History[len(dashboard.History)-1].FinishedAt)
	}
}

func TestBuildDashboardPreservesChronologicalOrder(t *testing.T) {
	existing := DashboardData{
		History: []DashboardHistoryEntry{
			{FinishedAt: "2026-03-18T00:00:10Z"},
			{FinishedAt: "2026-03-16T00:00:10Z"},
		},
	}
	stats := StatsDB{
		LastRun: LastRun{
			FinishedAt: "2026-03-17T00:00:10Z",
			OutputCounts: map[string]int{
				"all": 0,
			},
		},
	}

	dashboard := BuildDashboard(existing, stats, nil)

	want := []string{
		"2026-03-16T00:00:10Z",
		"2026-03-17T00:00:10Z",
		"2026-03-18T00:00:10Z",
	}
	for i, finishedAt := range want {
		if dashboard.History[i].FinishedAt != finishedAt {
			t.Fatalf("expected history[%d] to be %s, got %s", i, finishedAt, dashboard.History[i].FinishedAt)
		}
	}
}

func TestBuildDashboardSummary(t *testing.T) {
	stats := StatsDB{
		RunsTotal:           12,
		RequestsTotal:       321,
		ProxiesCheckedTotal: 210,
		ValidatedTotal:      44,
		LastSuccessAt:       "2026-03-17T23:13:32Z",
		LastRun: LastRun{
			FinishedAt:   "2026-03-18T00:00:00Z",
			Status:       "no_valid_proxies",
			SourceCounts: map[string]int{"repository": 10, "gist": 1},
			OutputCounts: map[string]int{"http": 7, "socks4": 2, "socks5": 3, "all": 12},
		},
	}

	summary := buildDashboardSummary(stats, []Proxy{
		{CountryCode: "US", Anonymity: AnonymityElite},
		{CountryCode: "", Anonymity: AnonymityUnknown},
		{CountryCode: "DE", Anonymity: AnonymityAnonymous},
	})

	if summary.Status != "no_valid_proxies" {
		t.Fatalf("expected status no_valid_proxies, got %s", summary.Status)
	}
	if summary.LastGenerated != "2026-03-18T00:00:00Z" {
		t.Fatalf("unexpected last generated: %s", summary.LastGenerated)
	}
	if summary.LastSuccessAt != "2026-03-17T23:13:32Z" {
		t.Fatalf("unexpected last success: %s", summary.LastSuccessAt)
	}
	if summary.CurrentOutputCounts["all"] != 12 {
		t.Fatalf("expected output count 12, got %d", summary.CurrentOutputCounts["all"])
	}
	if summary.CurrentSourceCounts["gist"] != 1 {
		t.Fatalf("expected gist count 1, got %d", summary.CurrentSourceCounts["gist"])
	}
	if summary.CurrentCountryCounts["US"] != 1 || summary.CurrentCountryCounts["unknown"] != 1 {
		t.Fatalf("unexpected country counts: %#v", summary.CurrentCountryCounts)
	}
	if summary.CurrentAnonymityCounts[string(AnonymityAnonymous)] != 1 {
		t.Fatalf("unexpected anonymity counts: %#v", summary.CurrentAnonymityCounts)
	}
}
