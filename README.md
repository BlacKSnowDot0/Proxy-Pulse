![Proxy Pulse](assets/banner.svg)

<h1 align="center">Proxy Pulse</h1>
<p align="center">Automated discovery, validation, and publishing of public proxy lists from GitHub repositories and gists.</p>

| Metric | Value |
| --- | ---: |
| Last run status | success |
| Last generated | 2026-03-17T13:44:58Z |
| Last successful refresh | 2026-03-17T13:44:58Z |
| Total runs | 7 |
| Total outbound requests | 99993 |
| Total proxies checked | 160149 |
| Total validated proxies | 15728 |

## Published Lists

| File | Description | Count |
| --- | --- | ---: |
| [http.txt](http.txt) | Validated HTTP proxies | 652 |
| [socks4.txt](socks4.txt) | Validated SOCKS4 proxies | 168 |
| [socks5.txt](socks5.txt) | Validated SOCKS5 proxies | 717 |
| [all.txt](all.txt) | Combined scheme-qualified list | 1537 |
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
