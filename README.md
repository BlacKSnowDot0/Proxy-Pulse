![Proxy Pulse](assets/banner.svg)

<h1 align="center">Proxy Pulse</h1>
<p align="center">Automated discovery, validation, and publishing of public proxy lists from GitHub repositories and gists.</p>

| Metric | Value |
| --- | ---: |
| Last run status | success |
| Last generated | 2026-03-17T02:49:22Z |
| Last successful refresh | 2026-03-17T02:49:22Z |
| Total runs | 5 |
| Total outbound requests | 65633 |
| Total proxies checked | 117570 |
| Total validated proxies | 12705 |

## Published Lists

| File | Description | Count |
| --- | --- | ---: |
| [http.txt](http.txt) | Validated HTTP proxies | 423 |
| [socks4.txt](socks4.txt) | Validated SOCKS4 proxies | 176 |
| [socks5.txt](socks5.txt) | Validated SOCKS5 proxies | 2043 |
| [all.txt](all.txt) | Combined scheme-qualified list | 2642 |
| [stats.json](stats.json) | Machine-readable run database | 1 |

## Workflow

1. Search public GitHub repositories and gists using common proxy queries.
2. Scan .txt files and proxy-named text files for candidate host:port pairs.
3. Deduplicate candidates, split them across validation shards, and check every proxy through a public IP-echo endpoint.
4. Merge shard results and regenerate the published lists, stats database, and this README.

## Notes

- Only proxies that pass the latest validation run are published.
- If a run finds zero valid proxies, the last known good published lists are preserved.
- Public proxies are unstable; treat every entry as disposable infrastructure.
