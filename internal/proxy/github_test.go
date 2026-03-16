package proxy

import "testing"

func TestExtractGistIDs(t *testing.T) {
	body := `
<a href="/alice/0123456789abcdef0123">one</a>
<a href="/bob/abcdefabcdefabcdefabcd">two</a>
<a href="/alice/0123456789abcdef0123">repeat</a>
`

	got := extractGistIDs(body, 10)
	if len(got) != 2 {
		t.Fatalf("expected 2 gist ids, got %d", len(got))
	}
}
