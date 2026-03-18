package proxy

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
)

const dashboardHistoryLimit = 180

type DashboardData struct {
	GeneratedAt string                  `json:"generated_at"`
	Summary     DashboardSummary        `json:"summary"`
	History     []DashboardHistoryEntry `json:"history"`
}

type DashboardSummary struct {
	Status              string         `json:"status"`
	LastGenerated       string         `json:"last_generated"`
	LastSuccessAt       string         `json:"last_success_at"`
	RunsTotal           int            `json:"runs_total"`
	RequestsTotal       int64          `json:"requests_total"`
	ProxiesCheckedTotal int            `json:"proxies_checked_total"`
	ValidatedTotal      int            `json:"validated_total"`
	CurrentOutputCounts map[string]int `json:"current_output_counts"`
	CurrentSourceCounts map[string]int `json:"current_source_counts"`
}

type DashboardHistoryEntry struct {
	FinishedAt        string         `json:"finished_at"`
	Status            string         `json:"status"`
	RequestsMade      int64          `json:"requests_made"`
	SourcesScanned    int            `json:"sources_scanned"`
	FilesScanned      int            `json:"files_scanned"`
	CandidatesFound   int            `json:"candidates_found"`
	ProxiesChecked    int            `json:"proxies_checked"`
	Validated         int            `json:"validated"`
	DuplicatesRemoved int            `json:"duplicates_removed"`
	ErrorCount        int            `json:"error_count"`
	OutputCounts      map[string]int `json:"output_counts"`
	SourceCounts      map[string]int `json:"source_counts"`
}

func LoadDashboard(path string) (DashboardData, error) {
	var dashboard DashboardData

	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return dashboard, nil
	}
	if err != nil {
		return DashboardData{}, err
	}
	if err := json.Unmarshal(data, &dashboard); err != nil {
		return DashboardData{}, err
	}
	return dashboard, nil
}

func SaveDashboard(path string, dashboard DashboardData) error {
	return SaveJSON(path, dashboard)
}

func BuildDashboard(existing DashboardData, stats StatsDB) DashboardData {
	run := stats.LastRun
	history := append([]DashboardHistoryEntry(nil), existing.History...)
	if run.FinishedAt != "" {
		history = upsertDashboardHistory(history, dashboardHistoryEntry(run))
	}

	return DashboardData{
		GeneratedAt: run.FinishedAt,
		Summary:     buildDashboardSummary(stats),
		History:     history,
	}
}

func buildDashboardSummary(stats StatsDB) DashboardSummary {
	return DashboardSummary{
		Status:              stats.LastRun.Status,
		LastGenerated:       stats.LastRun.FinishedAt,
		LastSuccessAt:       stats.LastSuccessAt,
		RunsTotal:           stats.RunsTotal,
		RequestsTotal:       stats.RequestsTotal,
		ProxiesCheckedTotal: stats.ProxiesCheckedTotal,
		ValidatedTotal:      stats.ValidatedTotal,
		CurrentOutputCounts: dashboardOutputCounts(stats.LastRun.OutputCounts),
		CurrentSourceCounts: copyIntMap(stats.LastRun.SourceCounts),
	}
}

func dashboardHistoryEntry(run LastRun) DashboardHistoryEntry {
	return DashboardHistoryEntry{
		FinishedAt:        run.FinishedAt,
		Status:            run.Status,
		RequestsMade:      run.RequestsMade,
		SourcesScanned:    run.SourcesScanned,
		FilesScanned:      run.FilesScanned,
		CandidatesFound:   run.CandidatesFound,
		ProxiesChecked:    run.ProxiesChecked,
		Validated:         run.Validated,
		DuplicatesRemoved: run.DuplicatesRemoved,
		ErrorCount:        run.ErrorCount,
		OutputCounts:      dashboardOutputCounts(run.OutputCounts),
		SourceCounts:      copyIntMap(run.SourceCounts),
	}
}

func upsertDashboardHistory(history []DashboardHistoryEntry, entry DashboardHistoryEntry) []DashboardHistoryEntry {
	replaced := false
	for i := range history {
		if history[i].FinishedAt == entry.FinishedAt {
			history[i] = entry
			replaced = true
			break
		}
	}
	if !replaced {
		history = append(history, entry)
	}

	sort.Slice(history, func(i, j int) bool {
		return history[i].FinishedAt < history[j].FinishedAt
	})

	if len(history) > dashboardHistoryLimit {
		history = append([]DashboardHistoryEntry(nil), history[len(history)-dashboardHistoryLimit:]...)
	}
	return history
}

func dashboardOutputCounts(values map[string]int) map[string]int {
	counts := map[string]int{
		"http":   0,
		"socks4": 0,
		"socks5": 0,
		"all":    0,
	}
	for key, value := range values {
		counts[key] = value
	}
	return counts
}

func copyIntMap(values map[string]int) map[string]int {
	if len(values) == 0 {
		return map[string]int{}
	}
	out := make(map[string]int, len(values))
	for key, value := range values {
		out[key] = value
	}
	return out
}

func dashboardPath(outputDir string) string {
	return filepath.Join(outputDir, "docs", "data", "dashboard.json")
}
