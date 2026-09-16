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
| 🚦 Last run status | success_with_errors |
| 🕒 Last generated | 2026-09-16T21:14:49Z |
| ✅ Last successful refresh | 2026-09-16T21:14:49Z |
| 🔁 Total runs | 728 |
| 🌐 Total outbound requests | 107982184 |
| 🧪 Total proxies checked | 97483791 |
| 📡 Total validated proxies | 651759 |

## 📂 Published Lists

| File | Description | Count |
| --- | --- | ---: |
| [http.txt](http.txt) | Validated HTTP proxies | 55 |
| [https.txt](https.txt) | HTTP proxies with CONNECT tunnel support | 20 |
| [socks4.txt](socks4.txt) | Validated SOCKS4 proxies | 174 |
| [socks5.txt](socks5.txt) | Validated SOCKS5 proxies | 56 |
| [all.txt](all.txt) | Combined scheme-qualified list | 285 |
| [stats.json](stats.json) | Machine-readable run database | 1 |
| [docs/data/dashboard.json](docs/data/dashboard.json) | Machine-readable dashboard dataset | 1 |
| [docs/data/proxies.json](docs/data/proxies.json) | Machine-readable validated proxy metadata | 285 |
| [sources.txt](sources.txt) | Discovered source file URLs | fresh each run |
| [data/sources.json](data/sources.json) | Discovered source database | capped at 2000 |
| [data/known-good.json](data/known-good.json) | Persistent proxy reliability state | rolling |

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
