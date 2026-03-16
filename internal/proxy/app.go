package proxy

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"
)

func Run(ctx context.Context, cfg Config) error {
	if cfg.ValidationTimeout <= 0 {
		return errors.New("validation timeout must be positive")
	}

	statsPath := filepath.Join(cfg.OutputDir, "stats.json")
	stats, err := LoadStats(statsPath)
	if err != nil {
		return fmt.Errorf("load stats: %w", err)
	}

	startedAt := time.Now().UTC()
	counter := &RequestCounter{}
	client := NewGitHubClient(cfg, counter)

	log.Printf("starting updater: queries=%d maxReposPerQuery=%d maxGistsPerQuery=%d maxFilesPerSource=%d maxCandidates=%d validationTimeout=%s concurrency=%d",
		len(cfg.Queries), cfg.MaxReposPerQuery, cfg.MaxGistsPerQuery, cfg.MaxFilesPerSource, cfg.MaxCandidates, cfg.ValidationTimeout, cfg.Concurrency)

	discovery, err := client.Discover(ctx)
	if err != nil {
		return fmt.Errorf("discover sources: %w", err)
	}
	log.Printf("discovery complete: sources=%d files=%d discoveryErrors=%d", len(uniqueSources(discovery.Files)), len(discovery.Files), discovery.ErrorCount)

	run := LastRun{
		StartedAt:    startedAt.Format(time.RFC3339),
		Status:       "success",
		SourceCounts: discovery.SourceCounts,
	}

	var candidates []Candidate
	run.SourcesScanned = len(uniqueSources(discovery.Files))
	for _, file := range discovery.Files {
		content, err := client.FetchText(ctx, file)
		if err != nil {
			run.ErrorCount++
			continue
		}
		run.FilesScanned++

		extracted := ExtractCandidates(content, file.Path, file.SourceURL)
		run.CandidatesFound += len(extracted)
		candidates = append(candidates, extracted...)
	}
	log.Printf("extraction complete: filesScanned=%d rawCandidates=%d", run.FilesScanned, len(candidates))

	merged, duplicatesRemoved := MergeCandidates(candidates)
	run.DuplicatesRemoved = duplicatesRemoved
	if cfg.MaxCandidates > 0 && len(merged) > cfg.MaxCandidates {
		log.Printf("candidate cap applied: validating top %d of %d deduped candidates", cfg.MaxCandidates, len(merged))
		merged = merged[:cfg.MaxCandidates]
	}
	log.Printf("validation starting: dedupedCandidates=%d", len(merged))

	validator := NewValidator(cfg, counter)
	validationCtx := ctx
	var cancel context.CancelFunc
	if cfg.ValidationStageTimeout > 0 {
		validationCtx, cancel = context.WithTimeout(ctx, cfg.ValidationStageTimeout)
		defer cancel()
	}

	validated, checked, validationErrors := validator.ValidateAll(validationCtx, merged)
	run.ProxiesChecked = checked
	run.Validated = len(validated)
	run.ErrorCount += discovery.ErrorCount + validationErrors
	if validationCtx.Err() == context.DeadlineExceeded {
		run.ErrorCount++
		if run.Status == "success" {
			run.Status = "validation_stage_timeout"
		}
		log.Printf("validation stage reached timeout after %s; publishing partial results", cfg.ValidationStageTimeout)
	}
	log.Printf("validation complete: checked=%d validated=%d validationErrors=%d", checked, len(validated), validationErrors)

	outputCounts, err := PublishOutputs(cfg, validated)
	if err != nil {
		return fmt.Errorf("publish outputs: %w", err)
	}
	run.OutputCounts = outputCounts

	if len(validated) == 0 {
		run.Status = "no_valid_proxies"
	}
	if run.ErrorCount > 0 && run.Status == "success" {
		run.Status = "success_with_errors"
	}

	run.RequestsMade = counter.Load()
	run.FinishedAt = time.Now().UTC().Format(time.RFC3339)
	log.Printf("publishing complete: status=%s requests=%d http=%d socks4=%d socks5=%d all=%d",
		run.Status,
		run.RequestsMade,
		run.OutputCounts["http"],
		run.OutputCounts["socks4"],
		run.OutputCounts["socks5"],
		run.OutputCounts["all"],
	)

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

	return nil
}

func ensureBanner(outputDir string) error {
	assetsDir := filepath.Join(outputDir, "assets")
	if err := os.MkdirAll(assetsDir, 0o755); err != nil {
		return err
	}
	path := filepath.Join(assetsDir, "banner.svg")
	if _, err := os.Stat(path); err == nil {
		return nil
	}
	return os.WriteFile(path, []byte(defaultBannerSVG), 0o644)
}

func uniqueSources(files []SourceFile) map[string]struct{} {
	set := make(map[string]struct{})
	for _, file := range files {
		set[file.SourceType+":"+file.SourceID] = struct{}{}
	}
	return set
}

const defaultBannerSVG = `<svg width="1280" height="360" viewBox="0 0 1280 360" xmlns="http://www.w3.org/2000/svg" role="img" aria-labelledby="title desc">
  <title id="title">Proxy Pulse</title>
  <desc id="desc">Automated GitHub proxy discovery and validation dashboard banner.</desc>
  <defs>
    <linearGradient id="bg" x1="72" y1="44" x2="1210" y2="312" gradientUnits="userSpaceOnUse">
      <stop stop-color="#0B3B66"/>
      <stop offset="0.55" stop-color="#0F766E"/>
      <stop offset="1" stop-color="#F59E0B"/>
    </linearGradient>
  </defs>
  <rect width="1280" height="360" rx="30" fill="#07111F"/>
  <rect x="20" y="20" width="1240" height="320" rx="24" fill="url(#bg)"/>
  <g opacity="0.16" fill="#F8FAFC">
    <circle cx="1070" cy="92" r="46"/>
    <circle cx="1136" cy="242" r="70"/>
    <circle cx="184" cy="86" r="34"/>
  </g>
  <g transform="translate(100 92)">
    <rect x="0" y="0" width="162" height="162" rx="32" fill="#E5F4FF"/>
    <rect x="34" y="34" width="94" height="94" rx="20" fill="#0A2540"/>
    <path d="M81 48v66" stroke="#F8FAFC" stroke-width="14" stroke-linecap="round"/>
    <path d="M48 81h66" stroke="#F8FAFC" stroke-width="14" stroke-linecap="round"/>
    <circle cx="81" cy="81" r="52" stroke="#67E8F9" stroke-width="10" fill="none"/>
  </g>
  <text x="330" y="146" fill="#F8FAFC" font-size="64" font-weight="700" font-family="Segoe UI, Arial, sans-serif">Proxy Pulse</text>
  <text x="330" y="198" fill="#E2E8F0" font-size="28" font-family="Segoe UI, Arial, sans-serif">GitHub-powered proxy aggregation, validation, and publishing.</text>
  <g transform="translate(330 234)">
    <rect width="184" height="48" rx="24" fill="#082F49"/>
    <text x="24" y="31" fill="#BAE6FD" font-size="21" font-family="Segoe UI, Arial, sans-serif">Auto-refresh via Actions</text>
  </g>
  <g transform="translate(534 234)">
    <rect width="148" height="48" rx="24" fill="#134E4A"/>
    <text x="24" y="31" fill="#CCFBF1" font-size="21" font-family="Segoe UI, Arial, sans-serif">HTTP / SOCKS</text>
  </g>
  <g transform="translate(702 234)">
    <rect width="198" height="48" rx="24" fill="#7C2D12"/>
    <text x="24" y="31" fill="#FFEDD5" font-size="21" font-family="Segoe UI, Arial, sans-serif">Readable stats output</text>
  </g>
</svg>
`
