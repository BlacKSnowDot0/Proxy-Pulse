package proxy

import (
	"context"
	"os"
	"path/filepath"
)

func Run(ctx context.Context, cfg Config) error {
	manifest, candidates, err := DiscoverCandidates(ctx, cfg)
	if err != nil {
		return err
	}
	if cfg.MaxCandidates > 0 && len(candidates) > cfg.MaxCandidates {
		candidates = candidates[:cfg.MaxCandidates]
	}
	shardResult, err := ValidateShard(ctx, cfg, candidates, 0, 1)
	if err != nil {
		return err
	}
	return FinalizeRun(cfg, manifest, []ShardResult{shardResult})
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
      <stop stop-color="#0B1220"/>
      <stop offset="0.58" stop-color="#1E2A3D"/>
      <stop offset="1" stop-color="#B8934E"/>
    </linearGradient>
  </defs>
  <rect width="1280" height="360" rx="30" fill="#040814"/>
  <rect x="20" y="20" width="1240" height="320" rx="24" fill="url(#bg)"/>
  <g opacity="0.16" fill="#F4E8CF">
    <circle cx="1070" cy="92" r="46"/>
    <circle cx="1136" cy="242" r="70"/>
    <circle cx="184" cy="86" r="34"/>
  </g>
  <g transform="translate(100 92)">
    <rect x="0" y="0" width="162" height="162" rx="32" fill="#F4E8CF"/>
    <rect x="34" y="34" width="94" height="94" rx="20" fill="#0F172A"/>
    <path d="M81 48v66" stroke="#FAF7F2" stroke-width="14" stroke-linecap="round"/>
    <path d="M48 81h66" stroke="#FAF7F2" stroke-width="14" stroke-linecap="round"/>
    <circle cx="81" cy="81" r="52" stroke="#D4B06A" stroke-width="10" fill="none"/>
  </g>
  <text x="330" y="146" fill="#FBF7F0" font-size="64" font-weight="700" font-family="Segoe UI, Arial, sans-serif">Proxy Pulse</text>
  <text x="330" y="198" fill="#E8DDCA" font-size="28" font-family="Segoe UI, Arial, sans-serif">GitHub-powered proxy aggregation, validation, and publishing.</text>
  <g transform="translate(330 234)">
    <rect width="244" height="48" rx="24" fill="#0F1C2E"/>
    <text x="122" y="31" text-anchor="middle" fill="#E8D6B4" font-size="20" font-family="Segoe UI, Arial, sans-serif">Auto-refresh via Actions</text>
  </g>
  <g transform="translate(594 234)">
    <rect width="172" height="48" rx="24" fill="#1E2A24"/>
    <text x="86" y="31" text-anchor="middle" fill="#E6DECB" font-size="20" font-family="Segoe UI, Arial, sans-serif">HTTP / SOCKS</text>
  </g>
  <g transform="translate(786 234)">
    <rect width="226" height="48" rx="24" fill="#3A241A"/>
    <text x="113" y="31" text-anchor="middle" fill="#F0D6B8" font-size="20" font-family="Segoe UI, Arial, sans-serif">Readable stats output</text>
  </g>
</svg>
`
