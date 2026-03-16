package proxy

import "testing"

func TestStatsApplyRun(t *testing.T) {
	var stats StatsDB
	run := LastRun{
		FinishedAt:        "2026-03-16T00:00:00Z",
		RequestsMade:      10,
		SourcesScanned:    3,
		FilesScanned:      7,
		CandidatesFound:   40,
		ProxiesChecked:    22,
		Validated:         5,
		DuplicatesRemoved: 8,
		ErrorCount:        2,
	}

	stats.ApplyRun(run)

	if stats.RunsTotal != 1 {
		t.Fatalf("expected 1 run, got %d", stats.RunsTotal)
	}
	if stats.ProxiesCheckedTotal != 22 {
		t.Fatalf("expected checked total 22, got %d", stats.ProxiesCheckedTotal)
	}
	if stats.LastSuccessAt != run.FinishedAt {
		t.Fatalf("expected last success %s, got %s", run.FinishedAt, stats.LastSuccessAt)
	}
}
