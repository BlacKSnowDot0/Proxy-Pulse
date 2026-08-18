package proxy

import (
	"context"
	"fmt"
	"hash/fnv"
	"log"
	"path/filepath"
	"time"
)

func DiscoverCandidates(ctx context.Context, cfg Config) (RunManifest, []Candidate, error) {
	if cfg.ValidationTimeout <= 0 {
		return RunManifest{}, nil, fmt.Errorf("validation timeout must be positive")
	}

	startedAt := time.Now().UTC()
	counter := &RequestCounter{}
	client := NewGitHubClient(cfg, counter)

	log.Printf("starting discovery: queries=%d maxReposPerQuery=%d maxGistsPerQuery=%d maxFilesPerSource=%d concurrency=%d shards=%d",
		len(cfg.Queries), cfg.MaxReposPerQuery, cfg.MaxGistsPerQuery, cfg.MaxFilesPerSource, cfg.Concurrency, cfg.ShardCount)

	discovery, err := client.Discover(ctx)
	if err != nil {
		return RunManifest{}, nil, fmt.Errorf("discover sources: %w", err)
	}
	log.Printf("discovery complete: sources=%d files=%d discoveryErrors=%d", len(uniqueSources(discovery.Files)), len(discovery.Files), discovery.ErrorCount)

	manifest := RunManifest{
		StartedAt:    startedAt.Format(time.RFC3339),
		Status:       "success",
		SourceCounts: discovery.SourceCounts,
		Salt:         shardSalt(startedAt),
	}

	if discovery.GistFailures > 0 && discovery.GistHits == 0 {
		manifest.Warnings = append(manifest.Warnings, "gist search failed on all queries")
	} else if discovery.GistHits == 0 && cfg.MaxGistsPerQuery > 0 {
		manifest.Warnings = append(manifest.Warnings, "gist search returned no results")
	}

	var candidates []Candidate
	manifest.SourcesScanned = len(uniqueSources(discovery.Files))
	for _, file := range discovery.Files {
		content, err := client.FetchText(ctx, file)
		if err != nil {
			manifest.ErrorCount++
			continue
		}
		manifest.FilesScanned++

		extracted := ExtractCandidates(content, file.Path, file.SourceURL)
		manifest.CandidatesFound += len(extracted)
		candidates = append(candidates, extracted...)
		manifest.Sources = appendSourceRecord(manifest.Sources, SourceRecord{
			Type:            file.SourceType,
			ID:              file.SourceID,
			URL:             file.SourceURL,
			Path:            file.Path,
			DownloadURL:     file.DownloadURL,
			CandidatesFound: len(extracted),
		})
	}
	log.Printf("extraction complete: filesScanned=%d rawCandidates=%d", manifest.FilesScanned, len(candidates))

	merged, duplicatesRemoved := MergeCandidates(candidates)
	manifest.DuplicatesRemoved = duplicatesRemoved
	manifest.RequestsMade = counter.Load()
	manifest.DiscoveryFinished = time.Now().UTC().Format(time.RFC3339)

	merged = applyFreshBudget(merged, cfg.MaxFreshCandidates)

	kg, err := LoadKnownGood(knownGoodPath(cfg))
	if err != nil {
		log.Printf("known-good database unavailable (%v); continuing without survivors", err)
	} else {
		var injected int
		merged, injected = kg.InjectSurvivors(merged, startedAt)
		if injected > 0 {
			log.Printf("injected known-good survivors: %d", injected)
		}
	}
	manifest.CandidateCount = len(merged)

	if manifest.ErrorCount > 0 {
		manifest.Status = "success_with_errors"
	}

	log.Printf("candidate set prepared: dedupedCandidates=%d duplicatesRemoved=%d survivors=%d", len(merged), duplicatesRemoved, countSurvivors(merged))
	return manifest, merged, nil
}

func shardSalt(startedAt time.Time) string {
	return startedAt.UTC().Format("2006-01-02")
}

func appendSourceRecord(records []SourceRecord, record SourceRecord) []SourceRecord {
	for i := range records {
		if records[i].DownloadURL == record.DownloadURL {
			records[i].CandidatesFound += record.CandidatesFound
			return records
		}
	}
	return append(records, record)
}

func applyFreshBudget(candidates []Candidate, maxFresh int) []Candidate {
	if maxFresh <= 0 || len(candidates) <= maxFresh {
		return candidates
	}
	survivors := make([]Candidate, 0)
	fresh := make([]Candidate, 0, maxFresh)
	for _, candidate := range candidates {
		if candidate.Survivor {
			survivors = append(survivors, candidate)
		} else if len(fresh) < maxFresh {
			fresh = append(fresh, candidate)
		}
	}
	return append(survivors, fresh...)
}

func countSurvivors(candidates []Candidate) int {
	count := 0
	for _, candidate := range candidates {
		if candidate.Survivor {
			count++
		}
	}
	return count
}

func ValidateShard(ctx context.Context, cfg Config, candidates []Candidate, shardIndex int, shardTotal int, salt string) (ShardResult, error) {
	if shardTotal < 1 {
		return ShardResult{}, fmt.Errorf("shard total must be positive")
	}
	if shardIndex < 0 || shardIndex >= shardTotal {
		return ShardResult{}, fmt.Errorf("shard index %d out of range for total %d", shardIndex, shardTotal)
	}

	shardCandidates := SelectCandidatesForShard(candidates, shardIndex, shardTotal, salt)
	log.Printf("shard %d/%d assigned candidates=%d salt=%q", shardIndex+1, shardTotal, len(shardCandidates), salt)

	counter := &RequestCounter{}
	validator := NewValidator(cfg, counter)
	validationCtx := ctx
	var cancel context.CancelFunc
	if cfg.ValidationStageTimeout > 0 {
		validationCtx, cancel = context.WithTimeout(ctx, cfg.ValidationStageTimeout)
		defer cancel()
	}

	validated, checked, validationErrors, outcomes := validator.ValidateAll(validationCtx, shardCandidates)
	status := "success"
	if validationCtx.Err() == context.DeadlineExceeded {
		status = "timeout"
		log.Printf("shard %d/%d validation stage reached timeout after %s; returning partial results", shardIndex+1, shardTotal, cfg.ValidationStageTimeout)
	}
	log.Printf("shard %d/%d complete: assigned=%d checked=%d validated=%d errors=%d status=%s degraded=%t",
		shardIndex+1, shardTotal, len(shardCandidates), checked, len(validated), validationErrors, status, validator.Degraded())

	return ShardResult{
		ShardIndex:   shardIndex,
		ShardTotal:   shardTotal,
		Assigned:     len(shardCandidates),
		Checked:      checked,
		Validated:    len(validated),
		ErrorCount:   validationErrors,
		RequestsMade: counter.Load(),
		Degraded:     validator.Degraded(),
		Proxies:      validated,
		Outcomes:     outcomes,
	}, nil
}

func FinalizeRun(ctx context.Context, cfg Config, manifest RunManifest, shardResults []ShardResult) error {
	finishedAt := time.Now().UTC()
	runWarnings := append([]string(nil), manifest.Warnings...)
	for _, result := range shardResults {
		if result.Degraded {
			runWarnings = append(runWarnings, fmt.Sprintf("shard %d validated without direct-exit comparison", result.ShardIndex))
		}
	}
	for _, warning := range runWarnings {
		log.Printf("warning: %s", warning)
	}

	statsPath := filepath.Join(cfg.OutputDir, "stats.json")
	stats, err := LoadStats(statsPath)
	if err != nil {
		return fmt.Errorf("load stats: %w", err)
	}
	dashboard, err := LoadDashboard(dashboardPath(cfg.OutputDir))
	if err != nil {
		return fmt.Errorf("load dashboard: %w", err)
	}

	mergedProxies := mergeProxyResults(shardResults)

	kgPath := knownGoodPath(cfg)
	kg, err := LoadKnownGood(kgPath)
	if err != nil {
		log.Printf("known-good database unavailable: %v", err)
		runWarnings = append(runWarnings, "known-good load failed")
		kg = KnownGoodDB{Version: knownGoodVersion, Entries: map[string]KnownGoodEntry{}}
	}

	enrichCtx, enrichCancel := context.WithTimeout(ctx, 30*time.Minute)
	enrichedProxies, enrichWarnings := EnrichProxies(enrichCtx, cfg, mergedProxies, kg)
	enrichCancel()
	runWarnings = append(runWarnings, enrichWarnings...)

	outputCounts, err := PublishOutputs(cfg, enrichedProxies)
	if err != nil {
		return fmt.Errorf("publish outputs: %w", err)
	}
	publishedProxies, err := LoadPublishedProxies(cfg.OutputDir)
	if err != nil {
		return fmt.Errorf("load published proxies: %w", err)
	}

	if err := updateKnownGoodForRun(kgPath, kg, shardResults, enrichedProxies, finishedAt); err != nil {
		log.Printf("known-good update failed: %v", err)
		runWarnings = append(runWarnings, "known-good update failed")
	}
	if err := publishSources(cfg, manifest, finishedAt); err != nil {
		log.Printf("publish sources: %v", err)
		runWarnings = append(runWarnings, "publish sources failed")
	}

	run := LastRun{
		StartedAt:         manifest.StartedAt,
		FinishedAt:        finishedAt.Format(time.RFC3339),
		Status:            manifest.Status,
		RequestsMade:      manifest.RequestsMade + sumShardRequests(shardResults),
		SourcesScanned:    manifest.SourcesScanned,
		FilesScanned:      manifest.FilesScanned,
		CandidatesFound:   manifest.CandidatesFound,
		ProxiesChecked:    sumShardChecked(shardResults),
		Validated:         len(mergedProxies),
		DuplicatesRemoved: manifest.DuplicatesRemoved,
		ErrorCount:        manifest.ErrorCount + sumShardErrors(shardResults),
		SourceCounts:      manifest.SourceCounts,
		OutputCounts:      outputCounts,
		Warnings:          runWarnings,
	}

	switch {
	case len(mergedProxies) == 0:
		run.Status = "no_valid_proxies"
	case run.ErrorCount > 0 && run.Status == "success":
		run.Status = "success_with_errors"
	}

	stats.ApplyRun(run)
	if err := SaveStats(statsPath, stats); err != nil {
		return fmt.Errorf("save stats: %w", err)
	}
	if err := SaveDashboard(dashboardPath(cfg.OutputDir), BuildDashboard(dashboard, stats, publishedProxies)); err != nil {
		return fmt.Errorf("save dashboard: %w", err)
	}
	if err := WriteReadme(cfg.OutputDir, stats); err != nil {
		return fmt.Errorf("write readme: %w", err)
	}
	if err := ensureBanner(cfg.OutputDir); err != nil {
		return fmt.Errorf("ensure banner: %w", err)
	}

	log.Printf("finalize complete: shards=%d checked=%d validated=%d requests=%d knownGood=%d http=%d socks4=%d socks5=%d all=%d",
		len(shardResults),
		run.ProxiesChecked,
		run.Validated,
		run.RequestsMade,
		len(kg.Entries),
		run.OutputCounts["http"],
		run.OutputCounts["socks4"],
		run.OutputCounts["socks5"],
		run.OutputCounts["all"],
	)
	return nil
}

func updateKnownGoodForRun(path string, kg KnownGoodDB, shardResults []ShardResult, validated []Proxy, finishedAt time.Time) error {
	outcomes := make([]ShardOutcome, 0)
	for _, result := range shardResults {
		outcomes = append(outcomes, result.Outcomes...)
	}

	kg.UpdateForRun(outcomes, validated, finishedAt)
	pruned := kg.Prune(finishedAt)
	if pruned > 0 {
		log.Printf("known-good pruned %d entries", pruned)
	}
	return SaveKnownGood(path, kg)
}

func SelectCandidatesForShard(candidates []Candidate, shardIndex int, shardTotal int, salt string) []Candidate {
	out := make([]Candidate, 0, len(candidates)/max(1, shardTotal)+1)
	for _, candidate := range candidates {
		if shardForCandidate(candidate, shardTotal, salt) == shardIndex {
			out = append(out, candidate)
		}
	}
	return out
}

func shardForCandidate(candidate Candidate, shardTotal int, salt string) int {
	hasher := fnv.New32a()
	_, _ = hasher.Write([]byte(candidate.Address()))
	if salt != "" {
		_, _ = hasher.Write([]byte("|"))
		_, _ = hasher.Write([]byte(salt))
	}
	return int(hasher.Sum32() % uint32(shardTotal))
}

func mergeProxyResults(results []ShardResult) []Proxy {
	collected := make([]Proxy, 0)
	for _, result := range results {
		collected = append(collected, result.Proxies...)
	}
	return mergeProxySlice(collected)
}

func sumShardChecked(results []ShardResult) int {
	total := 0
	for _, result := range results {
		total += result.Checked
	}
	return total
}

func sumShardErrors(results []ShardResult) int {
	total := 0
	for _, result := range results {
		total += result.ErrorCount
	}
	return total
}

func sumShardRequests(results []ShardResult) int64 {
	var total int64
	for _, result := range results {
		total += result.RequestsMade
	}
	return total
}

func max(a int, b int) int {
	if a > b {
		return a
	}
	return b
}
