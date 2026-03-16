package proxy

import "testing"

func TestSelectCandidatesForShardPartitionsAllCandidates(t *testing.T) {
	candidates := []Candidate{
		{Host: "1.1.1.1", Port: 80},
		{Host: "2.2.2.2", Port: 80},
		{Host: "3.3.3.3", Port: 80},
		{Host: "4.4.4.4", Port: 80},
	}

	seen := make(map[string]struct{})
	for shard := 0; shard < 3; shard++ {
		for _, candidate := range SelectCandidatesForShard(candidates, shard, 3) {
			key := candidate.Address()
			if _, ok := seen[key]; ok {
				t.Fatalf("candidate %s assigned to multiple shards", key)
			}
			seen[key] = struct{}{}
		}
	}

	if len(seen) != len(candidates) {
		t.Fatalf("expected all candidates assigned, got %d of %d", len(seen), len(candidates))
	}
}

func TestMergeProxyResultsDeduplicates(t *testing.T) {
	results := []ShardResult{
		{ShardIndex: 0, Proxies: []Proxy{{Protocol: ProtocolHTTP, Host: "1.1.1.1", Port: 80, Sources: []string{"a"}}}},
		{ShardIndex: 1, Proxies: []Proxy{{Protocol: ProtocolHTTP, Host: "1.1.1.1", Port: 80, Sources: []string{"b"}}}},
	}

	merged := mergeProxyResults(results)
	if len(merged) != 1 {
		t.Fatalf("expected 1 merged proxy, got %d", len(merged))
	}
	if len(merged[0].Sources) != 2 {
		t.Fatalf("expected merged sources, got %v", merged[0].Sources)
	}
}
