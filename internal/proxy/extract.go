package proxy

import (
	"net"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

var proxyPattern = regexp.MustCompile(`(?i)\b((?:\d{1,3}\.){3}\d{1,3}|(?:[a-z0-9-]+\.)+[a-z]{2,}|localhost):([0-9]{2,5})\b`)

func shouldInspectPath(path string) bool {
	base := strings.ToLower(filepath.Base(path))
	ext := strings.ToLower(filepath.Ext(base))

	if ext == ".txt" {
		return true
	}

	if strings.Contains(base, "proxy") || strings.Contains(base, "socks") || strings.Contains(base, "http") {
		switch ext {
		case "", ".txt", ".list", ".lst", ".csv", ".conf", ".json", ".md":
			return true
		}
	}

	return false
}

func inferProtocols(path string, content string) []Protocol {
	basis := strings.ToLower(path)
	if len(content) > 2048 {
		content = content[:2048]
	}
	basis += "\n" + strings.ToLower(content)

	switch {
	case strings.Contains(basis, "socks5"):
		return []Protocol{ProtocolSOCKS5}
	case strings.Contains(basis, "socks4"):
		return []Protocol{ProtocolSOCKS4}
	case strings.Contains(basis, "http") || strings.Contains(basis, "https"):
		return []Protocol{ProtocolHTTP}
	default:
		return append([]Protocol(nil), AllProtocols()...)
	}
}

func ExtractCandidates(content string, path string, source string) []Candidate {
	matches := proxyPattern.FindAllStringSubmatch(content, -1)
	if len(matches) == 0 {
		return nil
	}

	hints := inferProtocols(path, content)
	out := make([]Candidate, 0, len(matches))
	for _, match := range matches {
		host := normalizeHost(match[1])
		port, err := strconv.Atoi(match[2])
		if err != nil || !validPort(port) || !validHost(host) {
			continue
		}
		out = append(out, Candidate{
			Host:          host,
			Port:          port,
			HintProtocols: append([]Protocol(nil), hints...),
			Sources:       []string{source},
		})
	}
	return out
}

func MergeCandidates(items []Candidate) ([]Candidate, int) {
	if len(items) == 0 {
		return nil, 0
	}

	merged := make(map[string]Candidate, len(items))
	for _, item := range items {
		key := item.Address()
		if existing, ok := merged[key]; ok {
			existing.HintProtocols = mergeProtocolHints(existing.HintProtocols, item.HintProtocols)
			existing.Sources = mergeSources(existing.Sources, item.Sources)
			merged[key] = existing
			continue
		}
		item.HintProtocols = protocolList(item.HintProtocols)
		item.Sources = mergeSources(nil, item.Sources)
		merged[key] = item
	}

	out := make([]Candidate, 0, len(merged))
	for _, item := range merged {
		out = append(out, item)
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].Host == out[j].Host {
			return out[i].Port < out[j].Port
		}
		return out[i].Host < out[j].Host
	})

	return out, len(items) - len(out)
}

func validPort(port int) bool {
	return port >= 1 && port <= 65535
}

func validHost(host string) bool {
	if ip := net.ParseIP(host); ip != nil {
		return ip.To4() != nil
	}

	labels := strings.Split(host, ".")
	if len(labels) < 1 {
		return false
	}
	for _, label := range labels {
		if label == "" {
			return false
		}
		for i, r := range label {
			if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
				continue
			}
			if r == '-' && i != 0 && i != len(label)-1 {
				continue
			}
			return false
		}
	}
	return true
}
