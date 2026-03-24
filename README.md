![Proxy Pulse](assets/banner.svg)

<h1 align="center">Proxy Pulse</h1>
<p align="center">Automated discovery, validation, and publishing of public proxy lists from GitHub repositories and gists.</p>
<p align="center">
  <strong><a href="https://blacksnowdot0.github.io/Proxy-Pulse/">📊 Live Dashboard</a></strong>
  &nbsp;•&nbsp;
  <strong><a href="all.txt">📦 Download Latest List</a></strong>
</p>

## ✨ Snapshot

| Metric | Value |
| --- | ---: |
| 🚦 Last run status | success |
| 🕒 Last generated | 2026-03-24T07:22:01Z |
| ✅ Last successful refresh | 2026-03-24T07:22:01Z |
| 🔁 Total runs | 39 |
| 🌐 Total outbound requests | 4526173 |
| 🧪 Total proxies checked | 4487792 |
| 📡 Total validated proxies | 109304 |

## 📂 Published Lists

| File | Description | Count |
| --- | --- | ---: |
| [http.txt](http.txt) | Validated HTTP proxies | 60 |
| [socks4.txt](socks4.txt) | Validated SOCKS4 proxies | 80 |
| [socks5.txt](socks5.txt) | Validated SOCKS5 proxies | 447 |
| [all.txt](all.txt) | Combined scheme-qualified list | 587 |
| [stats.json](stats.json) | Machine-readable run database | 1 |
| [docs/data/dashboard.json](docs/data/dashboard.json) | Machine-readable dashboard dataset | 1 |
| [docs/data/proxies.json](docs/data/proxies.json) | Machine-readable validated proxy metadata | 587 |

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
