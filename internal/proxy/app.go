package proxy

import (
	"context"
	"errors"
	"fmt"
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

	discovery, err := client.Discover(ctx)
	if err != nil {
		return fmt.Errorf("discover sources: %w", err)
	}

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

	merged, duplicatesRemoved := MergeCandidates(candidates)
	run.DuplicatesRemoved = duplicatesRemoved

	validator := NewValidator(cfg, counter)
	validated, checked, validationErrors := validator.ValidateAll(ctx, merged)
	run.ProxiesChecked = checked
	run.Validated = len(validated)
	run.ErrorCount += discovery.ErrorCount + validationErrors

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

const defaultBannerSVG = `<svg width="1200" height="320" viewBox="0 0 1200 320" fill="none" xmlns="http://www.w3.org/2000/svg">
  <rect width="1200" height="320" rx="28" fill="#0F172A"/>
  <rect x="28" y="28" width="1144" height="264" rx="20" fill="url(#paint0_linear)"/>
  <circle cx="228" cy="160" r="88" fill="#F8FAFC" fill-opacity="0.12"/>
  <circle cx="952" cy="112" r="54" fill="#F8FAFC" fill-opacity="0.14"/>
  <circle cx="1016" cy="212" r="76" fill="#F8FAFC" fill-opacity="0.08"/>
  <path d="M170 104C170 88.536 182.536 76 198 76H260C275.464 76 288 88.536 288 104V216C288 231.464 275.464 244 260 244H198C182.536 244 170 231.464 170 216V104Z" fill="#E2E8F0"/>
  <path d="M234 112L234 208" stroke="#0F172A" stroke-width="18" stroke-linecap="round"/>
  <path d="M206 140L262 140" stroke="#0F172A" stroke-width="18" stroke-linecap="round"/>
  <path d="M206 180L262 180" stroke="#0F172A" stroke-width="18" stroke-linecap="round"/>
  <text x="360" y="144" fill="#F8FAFC" font-size="58" font-family="Verdana, Geneva, sans-serif" font-weight="700">Proxy Pulse</text>
  <text x="360" y="198" fill="#DBEAFE" font-size="28" font-family="Verdana, Geneva, sans-serif">GitHub-sourced proxy discovery and validation, rebuilt automatically.</text>
  <defs>
    <linearGradient id="paint0_linear" x1="68" y1="68" x2="1124" y2="252" gradientUnits="userSpaceOnUse">
      <stop stop-color="#0F766E"/>
      <stop offset="1" stop-color="#1D4ED8"/>
    </linearGradient>
  </defs>
</svg>
`
