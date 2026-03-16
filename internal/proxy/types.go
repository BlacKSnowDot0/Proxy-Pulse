package proxy

import (
	"fmt"
	"sort"
	"strings"
)

type Protocol string

const (
	ProtocolHTTP   Protocol = "http"
	ProtocolSOCKS4 Protocol = "socks4"
	ProtocolSOCKS5 Protocol = "socks5"
)

func AllProtocols() []Protocol {
	return []Protocol{ProtocolHTTP, ProtocolSOCKS4, ProtocolSOCKS5}
}

type Candidate struct {
	Host          string
	Port          int
	HintProtocols []Protocol
	Sources       []string
}

func (c Candidate) Address() string {
	return fmt.Sprintf("%s:%d", c.Host, c.Port)
}

type Proxy struct {
	Protocol Protocol `json:"protocol"`
	Host     string   `json:"host"`
	Port     int      `json:"port"`
	Sources  []string `json:"sources,omitempty"`
}

func (p Proxy) Address() string {
	return fmt.Sprintf("%s:%d", p.Host, p.Port)
}

func (p Proxy) URI() string {
	return fmt.Sprintf("%s://%s", p.Protocol, p.Address())
}

type SourceFile struct {
	SourceType  string
	SourceID    string
	SourceURL   string
	Path        string
	DownloadURL string
}

type DiscoveryResult struct {
	Files        []SourceFile
	SourceCounts map[string]int
	ErrorCount   int
}

type LastRun struct {
	StartedAt         string         `json:"started_at"`
	FinishedAt        string         `json:"finished_at"`
	Status            string         `json:"status"`
	RequestsMade      int64          `json:"requests_made"`
	SourcesScanned    int            `json:"sources_scanned"`
	FilesScanned      int            `json:"files_scanned"`
	CandidatesFound   int            `json:"candidates_found"`
	ProxiesChecked    int            `json:"proxies_checked"`
	Validated         int            `json:"validated"`
	DuplicatesRemoved int            `json:"duplicates_removed"`
	ErrorCount        int            `json:"error_count"`
	SourceCounts      map[string]int `json:"source_counts"`
	OutputCounts      map[string]int `json:"output_counts"`
}

type StatsDB struct {
	RunsTotal              int     `json:"runs_total"`
	RequestsTotal          int64   `json:"requests_total"`
	SourcesScannedTotal    int     `json:"sources_scanned_total"`
	FilesScannedTotal      int     `json:"files_scanned_total"`
	CandidatesFoundTotal   int     `json:"candidates_found_total"`
	ProxiesCheckedTotal    int     `json:"proxies_checked_total"`
	ValidatedTotal         int     `json:"validated_total"`
	DuplicatesRemovedTotal int     `json:"duplicates_removed_total"`
	ErrorsTotal            int     `json:"errors_total"`
	LastSuccessAt          string  `json:"last_success_at,omitempty"`
	LastRun                LastRun `json:"last_run"`
}

func protocolList(input []Protocol) []Protocol {
	if len(input) == 0 {
		return append([]Protocol(nil), AllProtocols()...)
	}

	seen := make(map[Protocol]struct{}, len(input))
	out := make([]Protocol, 0, len(input))
	for _, protocol := range input {
		if _, ok := seen[protocol]; ok {
			continue
		}
		seen[protocol] = struct{}{}
		out = append(out, protocol)
	}
	return out
}

func mergeProtocolHints(existing []Protocol, next []Protocol) []Protocol {
	set := make(map[Protocol]struct{}, len(existing)+len(next))
	for _, protocol := range existing {
		set[protocol] = struct{}{}
	}
	for _, protocol := range next {
		set[protocol] = struct{}{}
	}

	out := make([]Protocol, 0, len(set))
	for _, protocol := range AllProtocols() {
		if _, ok := set[protocol]; ok {
			out = append(out, protocol)
		}
	}
	return out
}

func mergeSources(existing []string, next []string) []string {
	set := make(map[string]struct{}, len(existing)+len(next))
	for _, source := range existing {
		if source == "" {
			continue
		}
		set[source] = struct{}{}
	}
	for _, source := range next {
		if source == "" {
			continue
		}
		set[source] = struct{}{}
	}

	out := make([]string, 0, len(set))
	for source := range set {
		out = append(out, source)
	}
	sort.Strings(out)
	return out
}

func normalizeHost(host string) string {
	return strings.ToLower(strings.TrimSpace(host))
}

func sortProxies(items []Proxy) {
	sort.Slice(items, func(i, j int) bool {
		if items[i].Protocol == items[j].Protocol {
			if items[i].Host == items[j].Host {
				return items[i].Port < items[j].Port
			}
			return items[i].Host < items[j].Host
		}
		return items[i].Protocol < items[j].Protocol
	})
}
