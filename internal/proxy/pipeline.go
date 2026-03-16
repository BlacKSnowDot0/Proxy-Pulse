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
	}
	log.Printf("extraction complete: filesScanned=%d rawCandidates=%d", manifest.FilesScanned, len(candidates))

	merged, duplicatesRemoved := MergeCandidates(candidates)
	manifest.DuplicatesRemoved = duplicatesRemoved
	manifest.CandidateCount = len(merged)
	manifest.RequestsMade = counter.Load()
	manifest.DiscoveryFinished = time.Now().UTC().Format(time.RFC3339)

	if manifest.ErrorCount > 0 {
		manifest.Status = "success_with_errors"
	}

	log.Printf("candidate set prepared: dedupedCandidates=%d duplicatesRemoved=%d", len(merged), duplicatesRemoved)
	return manifest, merged, nil
}

func ValidateShard(ctx context.Context, cfg Config, candidates []Candidate, shardIndex int, shardTotal int) (ShardResult, error) {
	if shardTotal < 1 {
		return ShardResult{}, fmt.Errorf("shard total must be positive")
	}
	if shardIndex < 0 || shardIndex >= shardTotal {
		return ShardResult{}, fmt.Errorf("shard index %d out of range for total %d", shardIndex, shardTotal)
	}

	shardCandidates := SelectCandidatesForShard(candidates, shardIndex, shardTotal)
	log.Printf("shard %d/%d assigned candidates=%d", shardIndex+1, shardTotal, len(shardCandidates))

	counter := &RequestCounter{}
	validator := NewValidator(cfg, counter)
	validationCtx := ctx
	var cancel context.CancelFunc
	if cfg.ValidationStageTimeout > 0 {
		validationCtx, cancel = context.WithTimeout(ctx, cfg.ValidationStageTimeout)
		defer cancel()
	}

	validated, checked, validationErrors := validator.ValidateAll(validationCtx, shardCandidates)
	status := "success"
	if validationCtx.Err() == context.DeadlineExceeded {
		status = "timeout"
		log.Printf("shard %d/%d validation stage reached timeout after %s; returning partial results", shardIndex+1, shardTotal, cfg.ValidationStageTimeout)
	}
	log.Printf("shard %d/%d complete: assigned=%d checked=%d validated=%d errors=%d status=%s",
		shardIndex+1, shardTotal, len(shardCandidates), checked, len(validated), validationErrors, status)

	return ShardResult{
		ShardIndex:   shardIndex,
		ShardTotal:   shardTotal,
		Assigned:     len(shardCandidates),
		Checked:      checked,
		Validated:    len(validated),
		ErrorCount:   validationErrors,
		RequestsMade: counter.Load(),
		Proxies:      validated,
	}, nil
}

func FinalizeRun(cfg Config, manifest RunManifest, shardResults []ShardResult) error {
	statsPath := filepath.Join(cfg.OutputDir, "stats.json")
	stats, err := LoadStats(statsPath)
	if err != nil {
		return fmt.Errorf("load stats: %w", err)
	}

	mergedProxies := mergeProxyResults(shardResults)
	outputCounts, err := PublishOutputs(cfg, mergedProxies)
	if err != nil {
		return fmt.Errorf("publish outputs: %w", err)
	}

	run := LastRun{
		StartedAt:         manifest.StartedAt,
		FinishedAt:        time.Now().UTC().Format(time.RFC3339),
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
	if err := WriteReadme(cfg.OutputDir, stats); err != nil {
		return fmt.Errorf("write readme: %w", err)
	}
	if err := ensureBanner(cfg.OutputDir); err != nil {
		return fmt.Errorf("ensure banner: %w", err)
	}

	log.Printf("finalize complete: shards=%d checked=%d validated=%d requests=%d http=%d socks4=%d socks5=%d all=%d",
		len(shardResults),
		run.ProxiesChecked,
		run.Validated,
		run.RequestsMade,
		run.OutputCounts["http"],
		run.OutputCounts["socks4"],
		run.OutputCounts["socks5"],
		run.OutputCounts["all"],
	)
	return nil
}

func SelectCandidatesForShard(candidates []Candidate, shardIndex int, shardTotal int) []Candidate {
	out := make([]Candidate, 0, len(candidates)/max(1, shardTotal)+1)
	for _, candidate := range candidates {
		if shardForCandidate(candidate, shardTotal) == shardIndex {
			out = append(out, candidate)
		}
	}
	return out
}

func shardForCandidate(candidate Candidate, shardTotal int) int {
	hasher := fnv.New32a()
	_, _ = hasher.Write([]byte(candidate.Address()))
	return int(hasher.Sum32() % uint32(shardTotal))
}

func mergeProxyResults(results []ShardResult) []Proxy {
	merged := make(map[string]Proxy)
	for _, result := range results {
		for _, proxy := range result.Proxies {
			key := proxy.URI()
			if existing, ok := merged[key]; ok {
				existing.Sources = mergeSources(existing.Sources, proxy.Sources)
				merged[key] = existing
				continue
			}
			proxy.Sources = mergeSources(nil, proxy.Sources)
			merged[key] = proxy
		}
	}

	out := make([]Proxy, 0, len(merged))
	for _, proxy := range merged {
		out = append(out, proxy)
	}
	sortProxies(out)
	return out
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
