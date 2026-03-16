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
	if len(candidates) != 3 {
		t.Fatalf("expected 3 raw candidates, got %d", len(candidates))
	}
	if candidates[0].HintProtocols[0] != ProtocolSOCKS5 {
		t.Fatalf("expected socks5 hint, got %v", candidates[0].HintProtocols)
	}

	merged, duplicates := MergeCandidates(candidates)
	if duplicates != 1 {
		t.Fatalf("expected 1 duplicate, got %d", duplicates)
	}
	if len(merged) != 2 {
		t.Fatalf("expected 2 merged candidates, got %d", len(merged))
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
