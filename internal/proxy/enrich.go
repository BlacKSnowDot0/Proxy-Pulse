package proxy

import (
	"context"
	"crypto/tls"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// EnrichProxies resolves geo/ASN data offline, folds known-good history into
// per-proxy metadata, and probes HTTPS (CONNECT) capability for HTTP proxies.
// It runs once per finalize and never fails the run; failures degrade fields.
func EnrichProxies(ctx context.Context, cfg Config, proxies []Proxy, kg KnownGoodDB) ([]Proxy, []string) {
	var warnings []string

	resolver := NewGeoResolver(cfg)
	if resolver == nil {
		warnings = append(warnings, "geoip enrichment unavailable")
	} else {
		defer func() {
			if err := resolver.Close(); err != nil {
				log.Printf("geoip resolver close: %v", err)
			}
		}()
	}

	return enrichProxies(ctx, cfg, proxies, kg, resolver), warnings
}

func enrichProxies(ctx context.Context, cfg Config, proxies []Proxy, kg KnownGoodDB, resolver GeoResolver) []Proxy {
	out := make([]Proxy, 0, len(proxies))
	geoCache := make(map[string]GeoLookup)

	for _, proxy := range proxies {
		if proxy.ExitIP != "" && resolver != nil {
			geo, cached := geoCache[proxy.ExitIP]
			if !cached {
				geo, _ = resolver.Lookup(proxy.ExitIP)
				geoCache[proxy.ExitIP] = geo
			}
			if geo.CountryCode != "" {
				proxy.CountryCode = geo.CountryCode
				proxy.CountryName = geo.CountryName
			}
			if geo.ASN != 0 {
				proxy.ASN = geo.ASN
				proxy.Org = geo.Org
			}
		}

		if entry, ok := kg.Entries[proxy.URI()]; ok {
			proxy.UptimePct = entry.UptimePct()
			proxy.AvgLatency = entry.AvgLatencyMS
			proxy.FirstSeenAt = entry.FirstSeen
			proxy.HTTPSOK = entry.HTTPSOK
		}

		if proxy.Protocol == ProtocolHTTP && !proxy.HTTPSOK {
			probeCtx, cancel := context.WithTimeout(ctx, enrichDeadline(cfg))
			proxy.HTTPSOK = probeHTTPS(probeCtx, cfg, proxy.Host, proxy.Port)
			cancel()
		}

		out = append(out, proxy)
	}
	return out
}

func enrichDeadline(cfg Config) time.Duration {
	if cfg.EnrichmentTimeout > 0 {
		return cfg.EnrichmentTimeout
	}
	return cfg.ValidationTimeout
}

func probeHTTPS(ctx context.Context, cfg Config, host string, port int) bool {
	target := strings.TrimSpace(cfg.HTTPSProbeURL)
	if target == "" {
		return false
	}
	targetURL, err := url.Parse(target)
	if err != nil || targetURL.Scheme != "https" || targetURL.Host == "" {
		return false
	}

	proxyURL, err := url.Parse("http://" + net.JoinHostPort(host, strconv.Itoa(port)))
	if err != nil {
		return false
	}

	deadline := enrichDeadline(cfg)
	transport := &http.Transport{
		Proxy:                 http.ProxyURL(proxyURL),
		DialContext:           (&net.Dialer{Timeout: deadline}).DialContext,
		TLSHandshakeTimeout:   deadline,
		ResponseHeaderTimeout: deadline,
		ForceAttemptHTTP2:     false,
	}
	if cfg.httpsTLSRoots != nil {
		transport.TLSClientConfig = &tls.Config{RootCAs: cfg.httpsTLSRoots}
	}
	defer transport.CloseIdleConnections()

	client := &http.Client{Timeout: deadline, Transport: transport}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL.String(), nil)
	if err != nil {
		return false
	}
	req.Header.Set("User-Agent", cfg.UserAgent)

	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return false
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 128))
	if err != nil {
		return false
	}
	_, err = parsePublicIPv4Body(body)
	return err == nil
}
