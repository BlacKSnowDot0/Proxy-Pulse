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
var separatedProxyPattern = regexp.MustCompile(`(?im)(?:^|[\s\[{(,;])["']?((?:\d{1,3}\.){3}\d{1,3}|(?:[a-z0-9-]+\.)+[a-z]{2,}|localhost)["']?\s*[,;|\t ]+\s*["']?([0-9]{2,5})["']?(?:$|[\s\]})#,;])`)
var jsonProxyPattern = regexp.MustCompile(`(?is)"(?:ip|host|server|address)"\s*:\s*"((?:\d{1,3}\.){3}\d{1,3}|(?:[a-z0-9-]+\.)+[a-z]{2,}|localhost)".{0,120}?"(?:port|proxy_port)"\s*:\s*"?([0-9]{2,5})"?|"(?:port|proxy_port)"\s*:\s*"?([0-9]{2,5})"?.{0,120}?"(?:ip|host|server|address)"\s*:\s*"((?:\d{1,3}\.){3}\d{1,3}|(?:[a-z0-9-]+\.)+[a-z]{2,}|localhost)"`)

var commonProxyPorts = map[int]int{
	80:   9,
	81:   5,
	83:   5,
	88:   4,
	3128: 10,
	4145: 9,
	8000: 7,
	8001: 6,
	8008: 6,
	8080: 10,
	8081: 8,
	8082: 7,
	8088: 7,
	8888: 9,
	1080: 10,
}

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
	hints := inferProtocols(path, content)
	out := make([]Candidate, 0)
	seen := make(map[string]struct{})

	for _, match := range proxyPattern.FindAllStringSubmatch(content, -1) {
		out = appendCandidate(out, seen, match[1], match[2], hints, source)
	}
	for _, match := range separatedProxyPattern.FindAllStringSubmatch(content, -1) {
		out = appendCandidate(out, seen, match[1], match[2], hints, source)
	}
	for _, match := range jsonProxyPattern.FindAllStringSubmatch(content, -1) {
		host := match[1]
		port := match[2]
		if host == "" {
			host = match[4]
			port = match[3]
		}
		out = appendCandidate(out, seen, host, port, hints, source)
	}
	return out
}

func appendCandidate(out []Candidate, seen map[string]struct{}, hostRaw string, portRaw string, hints []Protocol, source string) []Candidate {
	host := normalizeHost(hostRaw)
	port, err := strconv.Atoi(portRaw)
	if err != nil || !validPort(port) || !validHost(host) {
		return out
	}

	key := host + ":" + strconv.Itoa(port)
	if _, ok := seen[key]; ok {
		return out
	}
	seen[key] = struct{}{}

	return append(out, Candidate{
		Host:          host,
		Port:          port,
		HintProtocols: append([]Protocol(nil), hints...),
		Sources:       []string{source},
	})
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
		leftScore := candidateScore(out[i])
		rightScore := candidateScore(out[j])
		if leftScore != rightScore {
			return leftScore > rightScore
		}
		if len(out[i].Sources) != len(out[j].Sources) {
			return len(out[i].Sources) > len(out[j].Sources)
		}
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
	if host == "localhost" {
		return false
	}

	if ip := net.ParseIP(host); ip != nil {
		ipv4 := ip.To4()
		if ipv4 == nil {
			return false
		}
		return isPublicIPv4(ipv4)
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

func candidateScore(candidate Candidate) int {
	score := len(candidate.Sources) * 10
	score += commonProxyPorts[candidate.Port]
	if len(candidate.HintProtocols) == 1 {
		score += 6
	}
	if net.ParseIP(candidate.Host) != nil {
		score += 4
	}
	return score
}

func preferredProtocols(candidate Candidate) []Protocol {
	if len(candidate.HintProtocols) == 1 {
		return protocolList(candidate.HintProtocols)
	}

	switch candidate.Port {
	case 80, 81, 83, 88, 3128, 8000, 8001, 8008, 8080, 8081, 8082, 8088, 8888:
		return []Protocol{ProtocolHTTP, ProtocolSOCKS5, ProtocolSOCKS4}
	case 1080:
		return []Protocol{ProtocolSOCKS5, ProtocolSOCKS4, ProtocolHTTP}
	case 4145:
		return []Protocol{ProtocolSOCKS4, ProtocolSOCKS5, ProtocolHTTP}
	default:
		return protocolList(candidate.HintProtocols)
	}
}

func isPublicIPv4(ip net.IP) bool {
	if len(ip) != net.IPv4len {
		return false
	}

	switch {
	case ip[0] == 0:
		return false
	case ip[0] == 10:
		return false
	case ip[0] == 100 && ip[1] >= 64 && ip[1] <= 127:
		return false
	case ip[0] == 127:
		return false
	case ip[0] == 169 && ip[1] == 254:
		return false
	case ip[0] == 172 && ip[1] >= 16 && ip[1] <= 31:
		return false
	case ip[0] == 192 && ip[1] == 168:
		return false
	case ip[0] == 192 && ip[1] == 0 && ip[2] == 2:
		return false
	case ip[0] == 198 && ip[1] == 18:
		return false
	case ip[0] == 198 && ip[1] == 19:
		return false
	case ip[0] == 198 && ip[1] == 51 && ip[2] == 100:
		return false
	case ip[0] == 203 && ip[1] == 0 && ip[2] == 113:
		return false
	case ip[0] >= 224:
		return false
	default:
		return true
	}
}
