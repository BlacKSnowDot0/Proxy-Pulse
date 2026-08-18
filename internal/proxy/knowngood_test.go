package proxy

import (
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

func knownGoodFixture(now time.Time) KnownGoodDB {
	past := now.Add(-6 * time.Hour).Format(time.RFC3339)
	older := now.Add(-30 * time.Hour).Format(time.RFC3339)
	dead := now.Add(-20 * 24 * time.Hour).Format(time.RFC3339)
	return KnownGoodDB{
		Version:   1,
		UpdatedAt: past,
		Entries: map[string]KnownGoodEntry{
			"socks5://1.2.3.4:1080": {FirstSeen: past, LastSeen: past, LastPass: past, Protocol: "socks5", Results: "1111"},
			"http://5.6.7.8:8080":   {FirstSeen: older, LastSeen: older, LastPass: older, Protocol: "http", Results: "1101"},
			"http://9.9.9.9:3128":   {FirstSeen: dead, LastSeen: dead, Protocol: "http", Results: "1000000000"},
		},
	}
}

func TestKnownGoodInjectSurvivorsBoostsRecent(t *testing.T) {
	now := time.Now().UTC()
	db := knownGoodFixture(now)

	candidates := []Candidate{
		{Host: "5.6.7.8", Port: 8080, HintProtocols: AllProtocols()},
		{Host: "20.20.20.20", Port: 8080},
	}

	merged, injected := db.InjectSurvivors(candidates, now)
	if injected != 2 {
		t.Fatalf("expected 2 injected survivors (1 upgrade + 1 new), got %d", injected)
	}

	byAddress := make(map[string]Candidate)
	for _, candidate := range merged {
		byAddress[candidate.Address()] = candidate
	}

	existing := byAddress["5.6.7.8:8080"]
	if !existing.Survivor {
		t.Fatalf("expected existing candidate to be marked survivor")
	}
	if len(existing.HintProtocols) != 1 || existing.HintProtocols[0] != ProtocolHTTP {
		t.Fatalf("expected survivor protocol hint http, got %v", existing.HintProtocols)
	}

	fresh := byAddress["1.2.3.4:1080"]
	if !fresh.Survivor || fresh.Port != 1080 || len(fresh.HintProtocols) != 1 || fresh.HintProtocols[0] != ProtocolSOCKS5 {
		t.Fatalf("expected injected socks5 survivor, got %+v", fresh)
	}

	if _, ok := byAddress["9.9.9.9:3128"]; ok {
		t.Fatalf("expected stale entry (no recent pass) not injected")
	}
}

func TestKnownGoodUpdateForRunAppliesOutcomes(t *testing.T) {
	now := time.Now().UTC()
	db := knownGoodFixture(now)
	entry := db.Entries["socks5://1.2.3.4:1080"]

	outcomes := []ShardOutcome{
		{Address: "1.2.3.4:1080", OK: true, Protocol: "socks5", LatencyMS: 500},
		{Address: "5.6.7.8:8080", OK: false},
		{Address: "30.30.30.30:8080", OK: true, Protocol: "http", LatencyMS: 900},
	}
	validated := []Proxy{
		{Protocol: ProtocolSOCKS5, Host: "1.2.3.4", Port: 1080, ExitIP: "40.40.40.40", LatencyMS: 500, HTTPSOK: true},
		{Protocol: ProtocolHTTP, Host: "30.30.30.30", Port: 8080, ExitIP: "50.50.50.50", LatencyMS: 900},
	}

	db.UpdateForRun(outcomes, validated, now)

	updated := db.Entries["socks5://1.2.3.4:1080"]
	if updated.Results != entry.Results+"1" {
		t.Fatalf("expected appended success bit, got %s", updated.Results)
	}
	if updated.AvgLatencyMS == 0 {
		t.Fatalf("expected latency ema to start, got %d", updated.AvgLatencyMS)
	}

	failed := db.Entries["http://5.6.7.8:8080"]
	if failed.Results != "1101"+"0" {
		t.Fatalf("expected appended failure bit, got %s", failed.Results)
	}

	added := db.Entries["http://30.30.30.30:8080"]
	if added.Results != "1" || added.LastPass == "" || added.Protocol != "http" || added.ExitIP != "50.50.50.50" {
		t.Fatalf("expected new entry for first-time validated proxy, got %+v", added)
	}
}

func TestKnownGoodUpdateIgnoresUnassigned(t *testing.T) {
	now := time.Now().UTC()
	db := knownGoodFixture(now)
	before := db.Entries["http://9.9.9.9:3128"].Results

	db.UpdateForRun(nil, nil, now)

	if db.Entries["http://9.9.9.9:3128"].Results != before {
		t.Fatalf("expected untouched results for unassigned entry")
	}
}

func TestKnownGoodPruneRemovesDeadEntries(t *testing.T) {
	now := time.Now().UTC()
	db := knownGoodFixture(now)

	removed := db.Prune(now)
	if removed != 1 {
		t.Fatalf("expected 1 pruned entry, got %d", removed)
	}
	if _, ok := db.Entries["http://9.9.9.9:3128"]; ok {
		t.Fatalf("expected dead-streak entry pruned")
	}
}

func TestKnownGoodPruneEnforcesCap(t *testing.T) {
	now := time.Now().UTC()
	db := KnownGoodDB{Entries: map[string]KnownGoodEntry{}}
	recent := now.Add(-time.Hour).Format(time.RFC3339)
	for i := 0; i < knownGoodMaxEntries+50; i++ {
		host := strconv.Itoa(i>>24&0xff) + "." + strconv.Itoa(i>>16&0xff) + "." + strconv.Itoa(i>>8&0xff) + "." + strconv.Itoa(i&0xff)
		uri := "http://" + host + ":8080"
		results := "1"
		if i%2 == 0 {
			results = "1111111111"
		}
		db.Entries[uri] = KnownGoodEntry{FirstSeen: recent, LastSeen: recent, LastPass: recent, Protocol: "http", Results: results}
	}

	if removed := db.Prune(now); removed != 50 {
		t.Fatalf("expected 50 evicted entries, got %d", removed)
	}
	if len(db.Entries) != knownGoodMaxEntries {
		t.Fatalf("expected cap %d entries, got %d", knownGoodMaxEntries, len(db.Entries))
	}
}

func TestKnownGoodRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "known-good.json")
	now := time.Now().UTC()
	db := knownGoodFixture(now)

	if err := SaveKnownGood(path, db); err != nil {
		t.Fatalf("save: %v", err)
	}
	loaded, err := LoadKnownGood(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(loaded.Entries) != len(db.Entries) {
		t.Fatalf("expected %d entries after round trip, got %d", len(db.Entries), len(loaded.Entries))
	}
}

func TestApplyFreshBudgetKeepsSurvivors(t *testing.T) {
	candidates := []Candidate{
		{Host: "1.1.1.1", Port: 80},
		{Host: "2.2.2.2", Port: 80},
		{Host: "3.3.3.3", Port: 80, Survivor: true},
	}

	capped := applyFreshBudget(candidates, 1)
	if len(capped) != 2 {
		t.Fatalf("expected survivor plus single fresh candidate, got %d candidates", len(capped))
	}
	byAddress := make(map[string]Candidate)
	for _, candidate := range capped {
		byAddress[candidate.Address()] = candidate
	}
	if _, ok := byAddress["2.2.2.2:80"]; ok {
		t.Fatalf("expected lowest-ranked fresh candidate dropped")
	}
	if _, ok := byAddress["1.1.1.1:80"]; !ok {
		t.Fatalf("expected top-ranked fresh candidate kept")
	}
	if _, ok := byAddress["3.3.3.3:80"]; !ok {
		t.Fatalf("expected survivor kept")
	}
}

func TestUptimeAndStreakHelpers(t *testing.T) {
	if got := uptimeRatio("1110"); got != 0.75 {
		t.Fatalf("expected 0.75 uptime, got %f", got)
	}
	if got := trailingFailures("1110"); got != 1 {
		t.Fatalf("expected 1 trailing failure, got %d", got)
	}
	if got := trailingFailures("1111"); got != 0 {
		t.Fatalf("expected 0 trailing failures, got %d", got)
	}
	if got := emaLatency(0, 800); got != 800 {
		t.Fatalf("expected first ema sample, got %d", got)
	}
	if got := emaLatency(1000, 500); got != 850 {
		t.Fatalf("expected ema 850, got %d", got)
	}
	results := ""
	for i := 0; i < 40; i++ {
		results = appendResult(results, true)
	}
	if len(results) != knownGoodMaxResults {
		t.Fatalf("expected results capped at %d, got %d", knownGoodMaxResults, len(results))
	}
}
