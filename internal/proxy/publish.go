package proxy

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/template"
	"time"
)

type publishView struct {
	DashboardURL   string
	DatasetURL     string
	GeneratedAt    string
	LastSuccessAt  string
	Status         string
	RunsTotal      int
	RequestsTotal  int64
	CheckedTotal   int
	ValidatedTotal int
	PublishedHTTP  int
	PublishedS4    int
	PublishedS5    int
	PublishedAll   int
}

func PublishOutputs(cfg Config, proxies []Proxy) (map[string]int, error) {
	if err := os.MkdirAll(cfg.OutputDir, 0o755); err != nil {
		return nil, err
	}

	if len(proxies) == 0 {
		return readOutputCounts(cfg.OutputDir)
	}

	lines := map[string][]string{
		"http":   {},
		"socks4": {},
		"socks5": {},
		"all":    {},
	}

	for _, proxy := range proxies {
		switch proxy.Protocol {
		case ProtocolHTTP:
			lines["http"] = append(lines["http"], proxy.Address())
		case ProtocolSOCKS4:
			lines["socks4"] = append(lines["socks4"], proxy.Address())
		case ProtocolSOCKS5:
			lines["socks5"] = append(lines["socks5"], proxy.Address())
		}
		lines["all"] = append(lines["all"], proxy.URI())
	}

	for key := range lines {
		sort.Strings(lines[key])
		filename := key + ".txt"
		path := filepath.Join(cfg.OutputDir, filename)
		content := strings.Join(lines[key], "\n")
		if content != "" {
			content += "\n"
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			return nil, err
		}
	}

	if err := SaveJSON(publishedProxyDatasetPath(cfg.OutputDir), ProxyDataset{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		Count:       len(proxies),
		Proxies:     mergeProxySlice(proxies),
	}); err != nil {
		return nil, err
	}

	return readOutputCounts(cfg.OutputDir)
}

func readOutputCounts(outputDir string) (map[string]int, error) {
	out := map[string]int{
		"http":   0,
		"socks4": 0,
		"socks5": 0,
		"all":    0,
	}

	for _, name := range []string{"http", "socks4", "socks5", "all"} {
		path := filepath.Join(outputDir, name+".txt")
		data, err := os.ReadFile(path)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, err
		}

		lines := 0
		for _, line := range strings.Split(string(data), "\n") {
			if strings.TrimSpace(line) != "" {
				lines++
			}
		}
		out[name] = lines
	}

	return out, nil
}

func WriteReadme(outputDir string, stats StatsDB) error {
	view := publishView{
		DashboardURL:   "https://blacksnowdot0.github.io/Proxy-Pulse/",
		DatasetURL:     "docs/data/proxies.json",
		GeneratedAt:    stats.LastRun.FinishedAt,
		LastSuccessAt:  noneIfEmpty(stats.LastSuccessAt),
		Status:         noneIfEmpty(stats.LastRun.Status),
		RunsTotal:      stats.RunsTotal,
		RequestsTotal:  stats.RequestsTotal,
		CheckedTotal:   stats.ProxiesCheckedTotal,
		ValidatedTotal: stats.ValidatedTotal,
		PublishedHTTP:  stats.LastRun.OutputCounts["http"],
		PublishedS4:    stats.LastRun.OutputCounts["socks4"],
		PublishedS5:    stats.LastRun.OutputCounts["socks5"],
		PublishedAll:   stats.LastRun.OutputCounts["all"],
	}
	if view.GeneratedAt == "" {
		view.GeneratedAt = time.Now().UTC().Format(time.RFC3339)
	}

	tpl := template.Must(template.New("readme").Parse(readmeTemplate))
	var buf bytes.Buffer
	if err := tpl.Execute(&buf, view); err != nil {
		return err
	}

	path := filepath.Join(outputDir, "README.md")
	return os.WriteFile(path, buf.Bytes(), 0o644)
}

func LoadPublishedProxyDataset(path string) (ProxyDataset, error) {
	var dataset ProxyDataset

	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return dataset, nil
	}
	if err != nil {
		return ProxyDataset{}, err
	}
	if err := json.Unmarshal(data, &dataset); err != nil {
		return ProxyDataset{}, err
	}
	dataset.Proxies = mergeProxySlice(dataset.Proxies)
	return dataset, nil
}

func LoadPublishedProxies(outputDir string) ([]Proxy, error) {
	dataset, err := LoadPublishedProxyDataset(publishedProxyDatasetPath(outputDir))
	if err != nil {
		return nil, err
	}
	return dataset.Proxies, nil
}

func publishedProxyDatasetPath(outputDir string) string {
	return filepath.Join(outputDir, "docs", "data", "proxies.json")
}

func noneIfEmpty(value string) string {
	if strings.TrimSpace(value) == "" {
		return "n/a"
	}
	return value
}

const readmeTemplate = `![Proxy Pulse](assets/banner.svg)

<h1 align="center">Proxy Pulse</h1>
<p align="center">Automated discovery, validation, and publishing of public proxy lists from GitHub repositories and gists.</p>
<p align="center">
  <strong><a href="{{.DashboardURL}}">📊 Live Dashboard</a></strong>
  &nbsp;•&nbsp;
  <strong><a href="all.txt">📦 Download Latest List</a></strong>
</p>

## ✨ Snapshot

| Metric | Value |
| --- | ---: |
| 🚦 Last run status | {{.Status}} |
| 🕒 Last generated | {{.GeneratedAt}} |
| ✅ Last successful refresh | {{.LastSuccessAt}} |
| 🔁 Total runs | {{.RunsTotal}} |
| 🌐 Total outbound requests | {{.RequestsTotal}} |
| 🧪 Total proxies checked | {{.CheckedTotal}} |
| 📡 Total validated proxies | {{.ValidatedTotal}} |

## 📂 Published Lists

| File | Description | Count |
| --- | --- | ---: |
| [http.txt](http.txt) | Validated HTTP proxies | {{.PublishedHTTP}} |
| [socks4.txt](socks4.txt) | Validated SOCKS4 proxies | {{.PublishedS4}} |
| [socks5.txt](socks5.txt) | Validated SOCKS5 proxies | {{.PublishedS5}} |
| [all.txt](all.txt) | Combined scheme-qualified list | {{.PublishedAll}} |
| [stats.json](stats.json) | Machine-readable run database | 1 |
| [docs/data/dashboard.json](docs/data/dashboard.json) | Machine-readable dashboard dataset | 1 |
| [{{.DatasetURL}}]({{.DatasetURL}}) | Machine-readable validated proxy metadata | {{.PublishedAll}} |

## ⚙️ Workflow

1. Search public GitHub repositories and gists using common proxy queries.
2. Scan .txt files and proxy-named text files for candidate host:port pairs.
3. Deduplicate candidates, split them across validation shards, and verify every proxy through two public IP-echo endpoints before enriching the surviving exit IP with country and anonymity metadata.
4. Merge shard results and regenerate the published lists, stats database, and this README.

## 📝 Notes

- Only proxies that pass the latest validation run are published.
- If a run finds zero valid proxies, the last known good published lists are preserved.
- Per-proxy metadata is published in docs/data/proxies.json while the root .txt lists stay scheme-focused.
- Public proxies are unstable; treat every entry as disposable infrastructure.
`
