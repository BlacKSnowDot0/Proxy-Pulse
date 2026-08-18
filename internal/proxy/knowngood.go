package proxy

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	knownGoodVersion        = 1
	knownGoodMaxEntries     = 10000
	knownGoodMaxResults     = 30
	knownGoodDeadStreak     = 10
	knownGoodMaxAge         = 14 * 24 * time.Hour
	knownGoodSurvivorWindow = 48 * time.Hour
	knownGoodLatencyAlpha   = 0.3
)

type KnownGoodDB struct {
	Version   int                        `json:"version"`
	UpdatedAt string                     `json:"updated_at"`
	Entries   map[string]KnownGoodEntry `json:"entries"`
}

type KnownGoodEntry struct {
	FirstSeen    string   `json:"first_seen"`
	LastSeen     string   `json:"last_seen"`
	LastPass     string   `json:"last_pass,omitempty"`
	Protocol     string   `json:"protocol,omitempty"`
	ExitIP       string   `json:"exit_ip,omitempty"`
	Results      string   `json:"results,omitempty"`
	AvgLatencyMS int64    `json:"avg_latency_ms,omitempty"`
	HTTPSOK      bool     `json:"https_ok,omitempty"`
	Sources      []string `json:"sources,omitempty"`
}

func knownGoodPath(cfg Config) string {
	return filepath.Join(cfg.OutputDir, "data", "known-good.json")
}

func LoadKnownGood(path string) (KnownGoodDB, error) {
	var db KnownGoodDB
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return KnownGoodDB{Version: knownGoodVersion, Entries: map[string]KnownGoodEntry{}}, nil
	}
	if err != nil {
		return KnownGoodDB{}, err
	}
	if err := jsonUnmarshalStrict(data, &db); err != nil {
		return KnownGoodDB{}, fmt.Errorf("load known-good: %w", err)
	}
	if db.Entries == nil {
		db.Entries = map[string]KnownGoodEntry{}
	}
	if db.Version == 0 {
		db.Version = knownGoodVersion
	}
	return db, nil
}

func SaveKnownGood(path string, db KnownGoodDB) error {
	db.Version = knownGoodVersion
	return SaveJSON(path, db)
}

// InjectSurvivors merges known-good survivors (last_pass within the survivor
// window) on top of the fresh candidate set. Survivors get a single-protocol
// hint from their last successful run and are marked so the budget cap never
// drops them. Returns the merged list and the number of survivors injected.
func (db KnownGoodDB) InjectSurvivors(candidates []Candidate, now time.Time) ([]Candidate, int) {
	byAddress := make(map[string]int, len(candidates))
	for i, candidate := range candidates {
		byAddress[candidate.Address()] = i
	}

	injected := 0
	out := candidates
	for uri, entry := range db.Entries {
		if !entry.isSurvivor(now, knownGoodSurvivorWindow) {
			continue
		}
		protocol := Protocol(entry.Protocol)
		if protocol == "" || !isKnownProtocol(protocol) {
			continue
		}

		host, port, err := knownGoodAddress(uri, protocol)
		if err != nil {
			continue
		}

		if idx, ok := byAddress[host+":"+strconv.Itoa(port)]; ok {
			out[idx].Survivor = true
			out[idx].HintProtocols = []Protocol{protocol}
			injected++
			continue
		}

		candidate := Candidate{
			Host:          host,
			Port:          port,
			HintProtocols: []Protocol{protocol},
			Sources:       mergeSources(nil, entry.Sources),
			Survivor:      true,
		}
		out = append(out, candidate)
		byAddress[candidate.Address()] = len(out) - 1
		injected++
	}
	return out, injected
}

// UpdateForRun folds shard outcomes and validated proxies into the database.
// Assigned-but-failed candidates append a failure bit; candidates not assigned
// this run are left untouched (absence is not failure). Newly validated
// addresses become new entries.
func (db *KnownGoodDB) UpdateForRun(outcomes []ShardOutcome, validated []Proxy, runFinished time.Time) {
	if db.Entries == nil {
		db.Entries = map[string]KnownGoodEntry{}
	}

	outcomeByAddress := make(map[string]ShardOutcome, len(outcomes))
	for _, outcome := range outcomes {
		outcomeByAddress[outcome.Address] = outcome
	}

	now := runFinished.Format(time.RFC3339)

	for uri, entry := range db.Entries {
		host, port, err := knownGoodAddress(uri, Protocol(entry.Protocol))
		if err != nil {
			continue
		}
		address := host + ":" + strconv.Itoa(port)
		outcome, assigned := outcomeByAddress[address]
		if !assigned {
			continue
		}

		entry.LastSeen = now
		entry.Results = appendResult(entry.Results, outcome.OK)
		if outcome.OK {
			entry.LastPass = now
			if outcome.Protocol != "" {
				entry.Protocol = outcome.Protocol
			}
			if outcome.LatencyMS > 0 {
				entry.AvgLatencyMS = emaLatency(entry.AvgLatencyMS, outcome.LatencyMS)
			}
		}
		db.Entries[uri] = entry
		delete(outcomeByAddress, address)
	}

	proxyByKey := make(map[string]Proxy, len(validated))
	for _, proxy := range validated {
		proxyByKey[proxy.URI()] = proxy
	}

	for address, outcome := range outcomeByAddress {
		if !outcome.OK {
			continue
		}
		protocol := Protocol(outcome.Protocol)
		if protocol == "" || !isKnownProtocol(protocol) {
			continue
		}
		proxy, ok := proxyByKey[protocol.String()+"://"+address]
		if !ok {
			continue
		}
		uri := proxy.URI()
		entry := KnownGoodEntry{
			FirstSeen: now,
			LastSeen:  now,
			LastPass:  now,
			Protocol:  proxy.Protocol.String(),
			ExitIP:    proxy.ExitIP,
			Results:   "1",
			HTTPSOK:   proxy.HTTPSOK,
			Sources:   mergeSources(nil, proxy.Sources),
		}
		if proxy.LatencyMS > 0 {
			entry.AvgLatencyMS = proxy.LatencyMS
		}
		if existing, ok := db.Entries[uri]; ok {
			entry.FirstSeen = firstNonEmpty(existing.FirstSeen, now)
			entry.Results = appendResult(strings.TrimSuffix(existing.Results, ""), true)
			entry.Sources = mergeSources(existing.Sources, entry.Sources)
			entry.HTTPSOK = existing.HTTPSOK || proxy.HTTPSOK
		}
		db.Entries[uri] = entry
	}

	db.UpdatedAt = now
}

// Prune drops dead entries (long failure streaks, age) and enforces the entry cap.
func (db *KnownGoodDB) Prune(now time.Time) int {
	before := len(db.Entries)

	for uri, entry := range db.Entries {
		if trailingFailures(entry.Results) >= knownGoodDeadStreak {
			delete(db.Entries, uri)
			continue
		}
		lastSeen := parseTimeOr(entry.LastSeen, now)
		if now.Sub(lastSeen) > knownGoodMaxAge {
			delete(db.Entries, uri)
		}
	}

	if excess := len(db.Entries) - knownGoodMaxEntries; excess > 0 {
		type ranked struct {
			uri       string
			uptime    float64
			firstSeen time.Time
		}
		ranks := make([]ranked, 0, len(db.Entries))
		for uri, entry := range db.Entries {
			ranks = append(ranks, ranked{
				uri:       uri,
				uptime:    uptimeRatio(entry.Results),
				firstSeen: parseTimeOr(entry.FirstSeen, now),
			})
		}
		// evict lowest uptime first, then oldest first_seen
		for i := 0; i < len(ranks) && excess > 0; i++ {
			for j := i + 1; j < len(ranks); j++ {
				if ranks[j].uptime < ranks[i].uptime ||
					(ranks[j].uptime == ranks[i].uptime && ranks[j].firstSeen.Before(ranks[i].firstSeen)) {
					ranks[i], ranks[j] = ranks[j], ranks[i]
				}
			}
		}
		for i := 0; i < excess && i < len(ranks); i++ {
			delete(db.Entries, ranks[i].uri)
		}
	}

	return before - len(db.Entries)
}

func (e KnownGoodEntry) isSurvivor(now time.Time, window time.Duration) bool {
	if e.LastPass == "" {
		return false
	}
	lastPass := parseTimeOr(e.LastPass, time.Time{})
	if lastPass.IsZero() {
		return false
	}
	return now.Sub(lastPass) <= window
}

func (e KnownGoodEntry) UptimePct() int {
	return int(uptimeRatio(e.Results) * 100)
}

func knownGoodAddress(uri string, protocol Protocol) (string, int, error) {
	parsed, err := url.Parse(uri)
	if err != nil {
		return "", 0, err
	}
	host, port, err := netSplitHostPort(parsed.Host)
	if err != nil {
		return "", 0, err
	}
	if protocol.String() != parsed.Scheme {
		return "", 0, fmt.Errorf("scheme mismatch %q", parsed.Scheme)
	}
	return normalizeHost(host), port, nil
}

func isKnownProtocol(protocol Protocol) bool {
	switch protocol {
	case ProtocolHTTP, ProtocolSOCKS4, ProtocolSOCKS5:
		return true
	default:
		return false
	}
}

func appendResult(results string, ok bool) string {
	bit := "0"
	if ok {
		bit = "1"
	}
	results += bit
	if len(results) > knownGoodMaxResults {
		results = results[len(results)-knownGoodMaxResults:]
	}
	return results
}

func trailingFailures(results string) int {
	count := 0
	for i := len(results) - 1; i >= 0; i-- {
		if results[i] != '0' {
			break
		}
		count++
	}
	return count
}

func uptimeRatio(results string) float64 {
	if results == "" {
		return 0
	}
	passes := 0
	for i := 0; i < len(results); i++ {
		if results[i] == '1' {
			passes++
		}
	}
	return float64(passes) / float64(len(results))
}

func emaLatency(prev int64, next int64) int64 {
	if prev <= 0 {
		return next
	}
	merged := float64(prev)*(1-knownGoodLatencyAlpha) + float64(next)*knownGoodLatencyAlpha
	return int64(merged + 0.5)
}

func parseTimeOr(value string, fallback time.Time) time.Time {
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return fallback
	}
	return parsed
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func (p Protocol) String() string {
	return string(p)
}

func jsonUnmarshalStrict(data []byte, target any) error {
	return json.Unmarshal(data, target)
}
