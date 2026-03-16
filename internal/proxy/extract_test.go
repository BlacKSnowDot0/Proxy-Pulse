package proxy

import "testing"

func TestExtractCandidatesAndMerge(t *testing.T) {
	content := `
127.0.0.1:8080
127.0.0.1:8080
example.com:1080
bad.host:70000
`

	candidates := ExtractCandidates(content, "lists/socks5.txt", "https://example.test/source")
	if len(candidates) != 1 {
		t.Fatalf("expected 1 raw candidate, got %d", len(candidates))
	}
	if candidates[0].HintProtocols[0] != ProtocolSOCKS5 {
		t.Fatalf("expected socks5 hint, got %v", candidates[0].HintProtocols)
	}

	merged, duplicates := MergeCandidates(candidates)
	if duplicates != 0 {
		t.Fatalf("expected 0 duplicates, got %d", duplicates)
	}
	if len(merged) != 1 {
		t.Fatalf("expected 1 merged candidate, got %d", len(merged))
	}
}

func TestShouldInspectPath(t *testing.T) {
	tests := map[string]bool{
		"proxy.txt":        true,
		"nested/http.md":   true,
		"nested/readme.md": false,
		"proxy-list.json":  true,
		"notes.csv":        false,
	}

	for path, want := range tests {
		if got := shouldInspectPath(path); got != want {
			t.Fatalf("%s: expected %v, got %v", path, want, got)
		}
	}
}

func TestCandidateOrderingPrefersLikelyProxies(t *testing.T) {
	merged, _ := MergeCandidates([]Candidate{
		{Host: "8.8.8.8", Port: 9000, HintProtocols: []Protocol{ProtocolHTTP}, Sources: []string{"a"}},
		{Host: "1.1.1.1", Port: 8080, HintProtocols: []Protocol{ProtocolHTTP}, Sources: []string{"a", "b"}},
		{Host: "2.2.2.2", Port: 1080, HintProtocols: []Protocol{ProtocolSOCKS5}, Sources: []string{"a"}},
	})

	if len(merged) != 3 {
		t.Fatalf("expected 3 candidates, got %d", len(merged))
	}
	if merged[0].Address() != "1.1.1.1:8080" {
		t.Fatalf("expected highest-ranked candidate first, got %s", merged[0].Address())
	}
}

func TestPreferredProtocolsByPort(t *testing.T) {
	got := preferredProtocols(Candidate{Host: "1.1.1.1", Port: 1080})
	if len(got) < 2 || got[0] != ProtocolSOCKS5 {
		t.Fatalf("expected socks5-first ordering for 1080, got %v", got)
	}
}
