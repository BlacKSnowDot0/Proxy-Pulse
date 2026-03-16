package proxy

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

func LoadStats(path string) (StatsDB, error) {
	var stats StatsDB

	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return stats, nil
	}
	if err != nil {
		return StatsDB{}, err
	}
	if err := json.Unmarshal(data, &stats); err != nil {
		return StatsDB{}, err
	}
	return stats, nil
}

func SaveStats(path string, stats StatsDB) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(stats, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o644)
}

func (stats *StatsDB) ApplyRun(run LastRun) {
	stats.RunsTotal++
	stats.RequestsTotal += run.RequestsMade
	stats.SourcesScannedTotal += run.SourcesScanned
	stats.FilesScannedTotal += run.FilesScanned
	stats.CandidatesFoundTotal += run.CandidatesFound
	stats.ProxiesCheckedTotal += run.ProxiesChecked
	stats.ValidatedTotal += run.Validated
	stats.DuplicatesRemovedTotal += run.DuplicatesRemoved
	stats.ErrorsTotal += run.ErrorCount
	stats.LastRun = run

	if run.Validated > 0 {
		stats.LastSuccessAt = run.FinishedAt
	}
}
