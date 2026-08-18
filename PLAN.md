# Proxy-Pulse v2 — "Repair & Deepen" Plan (Approach A)

Status: draft for implementation · Aug 2026
Baseline audit: code frozen 2026-03-18; 617 bot runs since; yield 0.22%, gist discovery dead, proxies.json never committed since March.

---

## 1. Goals & non-goals

**Goals**

1. Fix all critical bugs from the audit (P0).
2. Persistent per-proxy state across runs (`data/known-good.json`) — revalidate survivors first; track uptime, latency, first/last seen.
3. Offline GeoIP (Country + ASN) via [obeliskdev/geovault](https://github.com/obeliskdev/geovault) — kill per-request ip-api.com lookups.
4. Better validation signals: CONNECT/HTTPS capability, latency, reliable anonymity classification.
5. Publish discovered source URLs (`sources.json` / `sources.txt`) — the "found URLs" file that was never implemented.
6. Keep infra at $0 (GitHub Actions only), keep the 6h cadence, bot never stops running for more than one cycle.

**Non-goals (deferred to v3 / approach B)**

- Always-on daemon, REST API, multi-vantage quorum.
- IPv6 exit support (echo endpoints are v4-only; documented constraint).
- Dynamic source discovery beyond GitHub search (subscription URLs of aggregators).

---

## 2. Research findings

### 2.1 GeoVault (obeliskdev/geovault)

- Typed GeoLite2 **City + ASN** lookups from local MMDB; no network I/O during lookups; atomic DB swap; SHA-256 verified downloads; race-tested.
- **Requires Go ≥ 1.26** (go.mod: `go 1.26.5`). We are on 1.22 → toolchain bump required (workflows + go.mod + local dev).
- **Zero tags/releases** → must pin a pseudo-version (`80c7ed0`, 2026-08-13). Author `tester2024`, 5 commits, 4 days old, 2 stars. **Supply-chain risk — see risk register.**
- Default DB URLs are `https://git.io/...` shortlinks. git.io is a deprecated GitHub service; it currently 301s to the P3TERX mirror. We will **not** rely on git.io: set explicit mirror URLs via options/env.

### 2.2 MMDB mirror (P3TERX/GeoLite.mmdb)

- Auto-updated via Actions every ~3 days; latest release 2026-08-16 (verified fresh).
- Assets: `GeoLite2-City.mmdb` 62.9 MB, `GeoLite2-ASN.mmdb` 11.5 MB, `GeoLite2-Country.mmdb` 8.3 MB.
- Direct asset URLs (no git.io): `https://github.com/P3TERX/GeoLite.mmdb/releases/latest/download/<name>`.
- GeoVault only supports City+ASN record shapes (not Country). Since enrichment moves to **finalize-only** (one download per run, ~300 unique exit IPs), 75 MB is acceptable — and cached between runs (§6).
- License: GeoLite2 EULA/CC-BY-4.0 — we consume lookups, do not redistribute the DB. Add attribution note to README.

### 2.3 The "found URLs" file

Confirmed: **never existed.** `git log -S sources` shows per-proxy `Sources []string` (repo URLs) inside `candidates.json` (ephemeral artifact) and `proxies.json` (uncommitted since March — audit bug #1). Nothing publishes the list of discovered source files. Spec'd in §5.

### 2.4 Validation cost model (from stats.json)

- 176k candidates checked per run ÷ 16 shards ≈ 11k/shard; at concurrency 80 × 2s timeout ≈ 5–8 min/shard. Shards have ~3× headroom.
- Fresh-candidate garbage rate is the dominant cost: 606k raw → 430k dupes removed → 176k unique → 380 valid. Ranking exists (`candidateScore`) but is **never enforced** in shard mode.

---

## 3. Architecture after v2

```
                ┌────────────────────────────────────────────────────────┐
                │ data/known-good.json  (committed, capped, single writer)│
                └──────▲───────────────────────────────────┬────────────┘
                       │ read (survivors + protocol hints)  │ write (streaks,
                       │                                    │ uptime, latency, ASN)
  ┌─────────┐   ┌──────┴──────┐   ┌──────────────┐   ┌──────▼───────┐   ┌──────────────┐
  │ discover │──▶│ candidates  │──▶│ validate ×16 │──▶│   finalize   │──▶│ commit & push│
  │ (+KG inj)│   │ .json       │   │ (salted,     │   │ (enrich: geo │   │ lists, proxies│
  └──────────┘   └─────────────┘   │  budget cap) │   │  +ASN+anon,  │   │ .json, sources│
                                    └──────────────┘   │  KG update)  │   │ .json, KG, stats│
                                                      └──────────────┘   └──────────────┘
```

Key invariants:

- Shards stay **stateless**; only finalize (single job) writes known-good — no merge conflicts.
- Survivors are content-hashed across shards like any candidate (no dedicated lane needed).
- All published JSON changes are **additive fields only** — old dashboard keeps working.

---

## 4. Workstreams

### P0 — Hygiene & critical fixes (ship first, ~half day)

| # | Fix | Where |
|---|-----|-------|
| 0.1 | Add `docs/data/proxies.json` to `git add` list (audit bug #1) | `update.yml:126` |
| 0.2 | `concurrency: group: update-proxies, cancel-in-progress: false` (bug #4) | `update.yml` |
| 0.3 | Push with rebase-retry ×3 (races with manual pushes) | `update.yml` finalize |
| 0.4 | `getDirectIP`: replace `sync.Once` with once + **retry (2 attempts, backoff) + re-fetch on hard failure**; on persistent failure mark run `degraded` instead of failing every candidate (bug #3) | `validate.go:307` |
| 0.5 | Gist scraping: keep regex scraper but **count "0 gists found" as a warning** surfaced in manifest; do not silently report success (bug #2 — full fix is P4-optional, HTML scraping of gist search is inherently fragile) | `github.go:148` |
| 0.6 | `go 1.22` → `go 1.26` in go.mod + both workflows (`ci.yml`, `update.yml` setup-go) — required by geovault (P2) | `go.mod`, workflows |
| 0.7 | Bound response bodies in `getBytes` (audit #15) | `github.go:302` |
| 0.8 | Move SOCKS counter increment before handshake (audit #14) | `validate.go:390` |
| 0.9 | Artifact `retention-days: 3` (avoid artifact bloat) | `update.yml` uploads |
| 0.10 | CI: add `go vet ./...` step | `ci.yml` |

**Acceptance:** one scheduled run completes green; proxies.json commits; two overlapping runs don't corrupt each other.

### P1 — Persistent known-good state + budget enforcement (core, ~1–2 days)

**New file `internal/proxy/knowngood.go`** — schema `data/known-good.json` (committed):

```jsonc
{
  "version": 1,
  "updated_at": "2026-08-18T00:00:00Z",
  "entries": {
    "socks5://1.2.3.4:1080": {
      "first_seen": "2026-08-01T00:00:00Z",
      "last_seen": "2026-08-18T00:00:00Z",   // appeared in candidate set
      "last_pass": "2026-08-18T00:00:00Z",   // validated OK
      "protocol": "socks5",                  // last working protocol → becomes hint
      "exit_ip": "5.6.7.8",
      "results": "1110111",                  // last ≤30 runs, newest last
      "avg_latency_ms": 812,                 // EMA α=0.3 over successful probes
      "https_ok": true,
      "sources": ["https://github.com/..."]
    }
  }
}
```

Rules:

- **Injection:** `DiscoverCandidates` unions survivors (entries with `last_pass` ≤ 48h) on top of fresh candidates; survivors carry `HintProtocols=[protocol]` (skips protocol ladder), get `+1000` score.
- **Budget:** new env `MAX_FRESH_CANDIDATES` (default 60000). Applied **after** survivor injection — ranking (`candidateScore`) finally matters (fixes audit #9). Survivors are never budget-capped.
- **Update (finalize only):** for every candidate that was assigned this run: pass → append `1` + update EMA/exit_ip; assigned-but-failed → append `0`; not seen → append `0` only if it was expected? No — not-seen entries get `last_seen` untouched and **no result char** (absence ≠ failure).
- **Prune:** drop when `consecutive 0s ≥ 10` (≈60h dead) OR `last_seen` > 14d OR entry count > 10 000 (evict lowest uptime, then oldest).
- **Zero-valid runs:** keep known-good untouched (streaks decay naturally); existing "preserve last known good lists" behavior unchanged.
- **Salted sharding (audit #5):** `shardForCandidate(addr, salt)` with `salt = manifest.started_at[:10]` (UTC date). `validate-shard` reads the manifest from the artifact (new `--manifest` flag, default `state/manifest.json`) — deterministic even across midnight UTC.
- Port-gate hostname candidates (audit #8): hostname (non-IP) hosts accepted only if port ∈ `commonProxyPorts` ∪ {9050, 1081, 8118, 5678, 23333}. IP-literal hosts keep any port.

**Acceptance:** run N+1 revalidates ≥90% of run N's survivors; checked/run drops ≤ ~70k; `data/known-good.json` committed each run; `results`/`uptime_pct` present in proxies.json.

### P2 — Offline GeoIP + ASN via geovault (finalize-only) (~1 day)

- New `internal/proxy/geo.go`: thin wrapper — the **only** file importing geovault.
  ```go
  type GeoResolver interface {
      Country(ip string) (code, name string)
      ASN(ip string) (asn uint32, org string)
  }
  ```
  `geovault.New(WithDataDir, WithCityDatabaseURL(mirror), WithASNDatabaseURL(mirror), WithAutoUpdate(false))`; `Update(ctx)` once per finalize run; lookups for unique exit IPs only.
- Remove `lookupCountry` per-proxy HTTP calls from `validate.go` (ip-api.com gone). Shards record raw `ExitIP` only; enrichment happens in finalize.
- proxies.json additive fields: `asn`, `org` (new), keep `country_code`/`country_name`.
- Workflow: `actions/cache` for `~/.cache/geovault` (or `GEOVAULT_DATA_DIR`) keyed on the mirror's release tag (`hashFiles` not applicable — key on `date +WW` ISO week; weekly refresh is plenty).
- Env: `GEOIP_CITY_URL` / `GEOIP_ASN_URL` defaulting to the explicit P3TERX release URLs (never git.io).
- Pin: `go get github.com/obeliskdev/geovault@80c7ed0` (pseudo-version; no tags exist). `go mod verify` + `govulncheck ./...` in CI.
- **Degradation mode:** if DB download fails → finalize logs warning, publishes without geo fields, run status `success_with_errors`. Never fails the run.

**Acceptance:** country_code coverage = 100% of published proxies (or empty only for lookup-miss IPs); zero ip-api requests; finalize runtime +≤2min.

### P3 — Better validation signals (~1 day)

- **HTTPS/CONNECT capability:** for validated `http` proxies, one extra probe: `GET https://api.ipify.org` through the proxy (Go transport auto-CONNECTs). Success → `https_ok: true`. Optional `https.txt` list (uri form `http://host:port` — scheme stays http, capability flag in metadata). Probe timeout 2s, failure = `false`, not an error.
- **Latency:** duration of the successful primary echo probe, stored as `latency_ms` in the run record; EMA persisted in known-good (P1); publish `avg_latency_ms`.
- **Anonymity (fixes 79% unknown):** check **only survivors** (post-validation, ~400 req/run not 176k). Chain: `ANON_CHECK_URL` → `ANON_CHECK_URL_SECONDARY` → unknown. Separate timeout env `ENRICHMENT_TIMEOUT` (default 8s; today it shares the 2s probe timeout — root cause of `unknown` flood). Any provider failure ⇒ `unknown`, never an error.
- `uptime_pct` computed from `results` bitset and published in proxies.json.

**Acceptance:** `unknown` anonymity ≤ 40% of http proxies; `https_ok` present for all http entries; latency fields populated.

### P4 — Publish discovered source URLs (the original "found URLs" idea) (~half day)

- Discovery records per-file counts: `manifest.Sources []SourceRecord`:
  ```go
  type SourceRecord struct {
      Type            string `json:"type"`           // repository | gist
      ID              string `json:"id"`             // owner/repo or gist id
      URL             string `json:"url"`            // human URL
      Path            string `json:"path"`           // file path in source
      DownloadURL     string `json:"download_url"`   // raw URL
      CandidatesFound int    `json:"candidates_found"`
      FirstSeen, LastSeen string `json:"first_seen"/"last_seen"`
  }
  ```
- Finalize merges into committed `data/sources.json` (cap 2000 by last_seen, then candidates_found) and writes human-readable `sources.txt` (one DownloadURL per line, deduped).
- README template row + workflow `git add` entries.
- Gist discovery note: scraper stays best-effort (P0.5 warning); replacing HTML scrape with authenticated Gist Search API (`GITHUB_TOKEN` already available) is a stretch here — do it if token budget allows (30 req/run).

**Acceptance:** sources.json/txt commit each run; a user can trace every published proxy → file → repo (per-proxy `sources` already carry this).

### P5 — Polish (optional, after a week of good runs)

- Dashboard: uptime/latency/ASN columns in a proxies table (data now actually fresh); sources count KPI.
- README v2 rewrite: describe state loop, fields, attribution (GeoLite2/P3TERX, geovault), data license notes.
- Gist Search API migration if not done in P4.
- Dynamic port allowlist fed from verified known-good ports.

---

## 5. File-by-file change map

| File | Changes |
|---|---|
| `go.mod` | go 1.26; + geovault pinned pseudo-version; + maxminddb (indirect) |
| `internal/proxy/knowngood.go` | NEW — load/save/inject/update/prune known-good |
| `internal/proxy/geo.go` | NEW — GeoResolver wrapper (only geovault import) |
| `internal/proxy/enrich.go` | NEW — finalize-time geo/ASN/anonymity/https_ok enrichment |
| `internal/proxy/pipeline.go` | survivor injection; budget after injection; salted sharding; SourceRecord plumbing; per-candidate outcome map → shard result (pass/fail per address for state update) |
| `internal/proxy/validate.go` | remove per-proxy geo lookup; direct-IP retry; latency capture; https probe; enrichment timeouts; counter fix |
| `internal/proxy/extract.go` | hostname port-gate; extended port list |
| `internal/proxy/types.go` | Proxy: `+asn, org, https_ok, latency_ms, uptime_pct, avg_latency_ms`; SourceRecord type |
| `internal/proxy/publish.go` | sources.json/txt; https.txt; README template rows |
| `internal/proxy/dashboard.go` | summary: `+sources_total, known_good_size` (additive) |
| `internal/proxy/config.go` | `+MAX_FRESH_CANDIDATES, ENRICHMENT_TIMEOUT, ANON_CHECK_URL_SECONDARY, GEOIP_CITY_URL, GEOIP_ASN_URL, GEOVAULT_DATA_DIR` |
| `cmd/proxy-updater/main.go` | validate-shard `--manifest` flag (salt source) |
| `update.yml` | concurrency, cache, git add list, retention, push retry, go 1.26 |
| `ci.yml` | go 1.26, vet, govulncheck |

Shard result needs a small schema addition: `outcomes: [{"address": "1.2.3.4:1080", "ok": true, "protocol": "socks5", "latency_ms": 812}]` for known-good updating — `Proxies []Proxy` alone can't express "assigned but failed".

---

## 6. Workflow deltas (update.yml)

```yaml
concurrency:
  group: update-proxies
  cancel-in-progress: false
# discover: env MAX_FRESH_CANDIDATES: "60000"
# validate:  --manifest state/manifest.json (salt); env as today
# finalize:
#   - name: GeoIP cache
#     uses: actions/cache@v4
#     with:
#       path: .geovault-cache
#       key: geovault-${{ steps.week.outputs.ww }}   # date -u +%G-W%V
#   - env: GEOVAULT_DATA_DIR: .geovault-cache
#   - git add: + docs/data/proxies.json data/known-good.json data/sources.json sources.txt https.txt
#   - push: fetch/heads + rebase + retry loop (3×, sleep 10s)
```

---

## 7. Risk register (hardened)

| Risk | Sev | Mitigation |
|---|---|---|
| geovault abandoned / malicious update | HIGH | Pin commit `80c7ed0` (immutable pseudo-version); isolated behind `GeoResolver` interface (swap = 1 file, ~50 lines with `oschwald/maxminddb-golang` + Country.mmdb 8 MB); `go mod verify`; govulncheck in CI; go.sum committed. No update without manual bump. |
| git.io shortlinks die | MED | Never used by us — explicit P3TERX release URLs in env/config. If mirror dies: swap env URLs (or MaxMind direct with license key). |
| P3TERX mirror stale (>60d) | MED | Log DB mtime in finalize; if stale, run continues with old DB (cached) + warning status. Country granularity tolerates weeks. |
| known-good.json grows / thrashes | MED | Hard cap 10k entries; prune rules; JSON ~2–4 MB worst case (fine for git). |
| Commit feedback loop (state file triggers update.yml) | LOW | push trigger already has `paths:` filter (cmd/internal/go.mod/workflow) — new data files are outside it. Verified in P0 acceptance run. |
| Echo endpoint blocks GitHub runner ranges | MED | Both echo URLs already env-configurable; monitor `primary_probe_failed` rate; if direct IP fetch works but >95% probes fail, mark run `degraded_sources` (new status) instead of silently publishing empty. |
| Zero-valid run wipes state | LOW | Explicit rule: known-good untouched on `no_valid_proxies`; streaks decay on subsequent failures instead. |
| Go 1.26 bump breaks local dev | LOW | Requires local toolchain ≥1.26 (`go env GOVERSION` check before implementing); setup-go handles CI. |
| MaxMind EULA | LOW | Lookups only, no redistribution of DB. Attribution added to README (P5). |
| Known-good poisoning (lying proxy passes 2 probes) | LOW | Exit must differ from direct IP + match on both probes (existing); uptime bitset makes single-run flukes visible; no reputation weight beyond recency. |

---

## 8. Test plan

- **Unit (new):**
  - `knowngood_test.go`: inject/update/prune/cap; results-bitset math; "not seen ≠ failure"; survivor protocol hint overrides ladder.
  - `pipeline_test.go` additions: salted shard assignment (same salt ⇒ stable; different salt ⇒ redistributes); budget applies to fresh only; outcome map correctness.
  - `extract_test.go` additions: hostname port-gating (accepts `host:1080`, rejects `node.js:20`).
  - `geo_test.go`: fake GeoResolver; degradation path (download failure ⇒ empty fields, no error).
  - `enrich_test.go` (P3): https_ok via httptest through local CONNECT proxy; anonymity classification with fake header-echo servers (test_helpers pattern already exists for local listeners).
- **Existing:** `go test ./...` must stay green after every phase (no big-bang).
- **E2E:** after each phase, `workflow_dispatch` on a branch with `MAX_FRESH_CANDIDATES=3000`, inspect artifacts + resulting commit before merging to main.
- **CI gates:** `go vet`, `go test -race ./internal/...`, govulncheck.

## 9. Rollout & success metrics

Order: P0 → P1 → P2 → P3 → P4 → (P5). Each phase is a standalone merge; the bot keeps its 6h cadence throughout.

| Metric | Now | Target after v2 |
|---|---|---|
| checked / run | 176k | ≤ 70k |
| validated / run | 380 | ≥ 380 (grow as state matures) |
| survivor revalidation hit-rate | n/a | ≥ 30% at T+6h |
| geo coverage | partial, rate-limited | 100% (offline) |
| anonymity unknown | 79% | ≤ 40% |
| known-good size | n/a | 1–3k steady state |
| run wall time | ~8 min | ≤ 12 min (finalize +enrichment) |
