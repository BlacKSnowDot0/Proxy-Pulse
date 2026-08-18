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
		survivors := make([]Candidate, 0)
		fresh := make([]Candidate, 0, cfg.MaxCandidates)
		for _, candidate := range candidates {
			if candidate.Survivor {
				survivors = append(survivors, candidate)
			} else if len(fresh) < cfg.MaxCandidates {
				fresh = append(fresh, candidate)
			}
		}
		candidates = append(survivors, fresh...)
	}
	shardResult, err := ValidateShard(ctx, cfg, candidates, 0, 1, manifest.Salt)
	if err != nil {
		return err
	}
	return FinalizeRun(ctx, cfg, manifest, []ShardResult{shardResult})
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

const defaultBannerSVG = `<svg width="1280" height="360" viewBox="0 0 1280 360" xmlns="http://www.w3.org/2000/svg" role="img" aria-labelledby="fTitle fDesc">
  <title id="fTitle">Proxy Pulse</title>
  <desc id="fDesc">Bento grid banner: wordmark card, live proxy count with trend bars, exit-country chips, published-by-protocol bars, and pipeline status card.</desc>
  <defs>
    <linearGradient id="fGold" x1="0" y1="0" x2="0" y2="1">
      <stop offset="0" stop-color="#F4E2B0"/>
      <stop offset="1" stop-color="#C89B4F"/>
    </linearGradient>
    <linearGradient id="fCard" x1="0" y1="0" x2="0" y2="1">
      <stop offset="0" stop-color="#171C24"/>
      <stop offset="1" stop-color="#12171F"/>
    </linearGradient>
  </defs>

  <rect width="1280" height="360" fill="#0D1117"/>

  <!-- grid: outer margin 32, gutter 16 · row1 y=32 h=140 · row2 y=188 h=140 -->

  <!-- card 1: wordmark -->
  <g>
    <rect x="32" y="32" width="416" height="140" rx="16" fill="url(#fCard)" stroke="#30363D"/>
    <text x="60" y="76" font-family="Segoe UI, Arial, sans-serif" font-size="11.5" font-weight="600" letter-spacing="3" fill="#8B949E">AUTOMATED PROXY INTELLIGENCE</text>
    <text x="56" y="128" font-family="Segoe UI, Arial, sans-serif" font-size="46" font-weight="700"><tspan fill="#E6EDF3">Proxy</tspan><tspan fill="url(#fGold)"> Pulse</tspan></text>
  </g>

  <!-- card 2: live count + trend -->
  <g>
    <rect x="464" y="32" width="280" height="140" rx="16" fill="url(#fCard)" stroke="#30363D"/>
    <text x="492" y="72" font-family="Segoe UI, Arial, sans-serif" font-size="11.5" letter-spacing="2.5" fill="#8B949E">LIVE PROXIES</text>
    <text x="488" y="124" font-family="Segoe UI, Arial, sans-serif" font-size="52" font-weight="700" fill="url(#fGold)">380</text>
    <g fill="#D4B06A">
      <rect x="612" y="110" width="7" height="14" rx="2" opacity="0.3"/>
      <rect x="625" y="102" width="7" height="22" rx="2" opacity="0.45"/>
      <rect x="638" y="94"  width="7" height="30" rx="2" opacity="0.6"/>
      <rect x="651" y="104" width="7" height="20" rx="2" opacity="0.5"/>
      <rect x="664" y="88"  width="7" height="36" rx="2" opacity="0.8"/>
      <rect x="677" y="82"  width="7" height="42" rx="2"/>
    </g>
  </g>

  <!-- card 3: countries -->
  <g>
    <rect x="760" y="32" width="488" height="140" rx="16" fill="url(#fCard)" stroke="#30363D"/>
    <text x="788" y="72" font-family="Segoe UI, Arial, sans-serif" font-size="11.5" letter-spacing="2.5" fill="#8B949E">EXIT COUNTRIES</text>
    <text x="784" y="124" font-family="Segoe UI, Arial, sans-serif" font-size="52" font-weight="700" fill="#E6EDF3">44</text>
    <g font-family="Consolas, Menlo, monospace" font-size="13.5">
      <rect x="908"  y="96" width="46" height="28" rx="8" fill="#0F1D30" stroke="#2A3D57"/><text x="931"  y="115" text-anchor="middle" fill="#D8C9A4">US</text>
      <rect x="962"  y="96" width="46" height="28" rx="8" fill="#0F1D30" stroke="#2A3D57"/><text x="985"  y="115" text-anchor="middle" fill="#D8C9A4">RU</text>
      <rect x="1016" y="96" width="46" height="28" rx="8" fill="#0F1D30" stroke="#2A3D57"/><text x="1039" y="115" text-anchor="middle" fill="#D8C9A4">SG</text>
      <rect x="1070" y="96" width="46" height="28" rx="8" fill="#0F1D30" stroke="#2A3D57"/><text x="1093" y="115" text-anchor="middle" fill="#D8C9A4">FR</text>
      <rect x="1124" y="96" width="46" height="28" rx="8" fill="#0F1D30" stroke="#2A3D57"/><text x="1147" y="115" text-anchor="middle" fill="#D8C9A4">DE</text>
      <rect x="1178" y="96" width="42" height="28" rx="8" fill="#D4B06A" fill-opacity="0.08" stroke="#D4B06A" stroke-opacity="0.45"/><text x="1199" y="115" text-anchor="middle" fill="#E9CB8E">+39</text>
    </g>
  </g>

  <!-- card 4: protocol split -->
  <g>
    <rect x="32" y="188" width="712" height="140" rx="16" fill="url(#fCard)" stroke="#30363D"/>
    <text x="60" y="222" font-family="Segoe UI, Arial, sans-serif" font-size="11.5" letter-spacing="2.5" fill="#8B949E">PUBLISHED BY PROTOCOL</text>

    <g font-family="Segoe UI, Arial, sans-serif" font-size="15">
      <text x="118" y="258" text-anchor="end" fill="#C9D1D9">HTTP</text>
      <rect x="134" y="246" width="520" height="14" rx="7" fill="#21262D"/>
      <rect x="134" y="246" width="402" height="14" rx="7" fill="#7BC6A4"/>

      <text x="118" y="286" text-anchor="end" fill="#C9D1D9">SOCKS4</text>
      <rect x="134" y="274" width="520" height="14" rx="7" fill="#21262D"/>
      <rect x="134" y="274" width="520" height="14" rx="7" fill="#E0A458"/>

      <text x="118" y="314" text-anchor="end" fill="#C9D1D9">SOCKS5</text>
      <rect x="134" y="302" width="520" height="14" rx="7" fill="#21262D"/>
      <rect x="134" y="302" width="227" height="14" rx="7" fill="#D97866"/>
    </g>
    <g font-family="Consolas, Menlo, monospace" font-size="15" fill="#E9CB8E" text-anchor="end">
      <text x="700" y="258">133</text>
      <text x="700" y="286">172</text>
      <text x="700" y="314">75</text>
    </g>
  </g>

  <!-- card 5: pipeline status -->
  <g>
    <rect x="760" y="188" width="488" height="140" rx="16" fill="url(#fCard)" stroke="#30363D"/>
    <text x="788" y="222" font-family="Segoe UI, Arial, sans-serif" font-size="11.5" letter-spacing="2.5" fill="#8B949E">PIPELINE STATUS</text>
    <circle cx="806" cy="264" r="5" fill="#3FB950"/>
    <circle cx="806" cy="264" r="11" fill="none" stroke="#3FB950" stroke-opacity="0.3"/>
    <text x="826" y="271" font-family="Segoe UI, Arial, sans-serif" font-size="20" font-weight="600" fill="#E6EDF3">healthy — refreshed every 6h</text>
    <text x="788" y="306" font-family="Consolas, Menlo, monospace" font-size="14" fill="#8B949E">617 runs · 90.8M probes · GitHub Actions</text>
  </g>
</svg>
`
