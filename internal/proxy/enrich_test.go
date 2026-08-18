package proxy

import (
	"context"
	"crypto/x509"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

type fakeGeoResolver struct {
	byIP map[string]GeoLookup
}

func (f fakeGeoResolver) Lookup(ip string) (GeoLookup, bool) {
	lookup, ok := f.byIP[ip]
	return lookup, ok
}

func (f fakeGeoResolver) Close() error { return nil }

func TestEnrichProxiesAppliesGeoAndKnownGood(t *testing.T) {
	cfg := Config{HTTPSProbeURL: "", EnrichmentTimeout: 2 * time.Second}
	now := time.Now().UTC()
	kg := KnownGoodDB{Entries: map[string]KnownGoodEntry{
		"socks5://1.2.3.4:1080": {FirstSeen: now.Format(time.RFC3339), Results: "1110", AvgLatencyMS: 700},
	}}
	proxies := []Proxy{
		{Protocol: ProtocolSOCKS5, Host: "1.2.3.4", Port: 1080, ExitIP: "8.8.8.8"},
		{Protocol: ProtocolHTTP, Host: "5.6.7.8", Port: 8080, ExitIP: "1.1.1.1"},
	}
	resolver := fakeGeoResolver{byIP: map[string]GeoLookup{
		"8.8.8.8": {CountryCode: "US", CountryName: "United States", ASN: 15169, Org: "Google LLC"},
	}}

	enriched := enrichProxies(context.Background(), cfg, proxies, kg, resolver)

	if enriched[0].CountryCode != "US" || enriched[0].ASN != 15169 || enriched[0].Org != "Google LLC" {
		t.Fatalf("expected geo fields on first proxy, got %+v", enriched[0])
	}
	if enriched[0].UptimePct != 75 || enriched[0].AvgLatency != 700 || enriched[0].FirstSeenAt == "" {
		t.Fatalf("expected known-good fields folded in, got %+v", enriched[0])
	}
	if enriched[1].CountryCode != "" || enriched[1].UptimePct != 0 {
		t.Fatalf("expected second proxy untouched (no geo hit, no kg entry), got %+v", enriched[1])
	}
}

func TestEnrichProxiesNilResolverLeavesGeoEmpty(t *testing.T) {
	cfg := Config{HTTPSProbeURL: ""}
	proxies := []Proxy{{Protocol: ProtocolHTTP, Host: "5.6.7.8", Port: 8080, ExitIP: "1.1.1.1"}}

	enriched := enrichProxies(context.Background(), cfg, proxies, KnownGoodDB{}, nil)
	if enriched[0].CountryCode != "" || enriched[0].ASN != 0 {
		t.Fatalf("expected empty geo without resolver, got %+v", enriched[0])
	}
}

func TestProbeHTTPSDetectsConnectSupport(t *testing.T) {
	tlsServer := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("8.8.4.4"))
	}))
	t.Cleanup(tlsServer.Close)

	echoServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodConnect {
			t.Errorf("expected CONNECT for https target, got %s", r.Method)
			http.Error(w, "expected CONNECT", http.StatusBadRequest)
			return
		}
		hijacker, ok := w.(http.Hijacker)
		if !ok {
			http.Error(w, "no hijack", http.StatusInternalServerError)
			return
		}
		clientConn, _, err := hijacker.Hijack()
		if err != nil {
			return
		}
		defer clientConn.Close()

		if _, err := clientConn.Write([]byte("HTTP/1.1 200 Connection established\r\n\r\n")); err != nil {
			return
		}

		targetConn, err := net.Dial("tcp", r.URL.Host)
		if err != nil {
			return
		}
		defer targetConn.Close()
		relay(clientConn, targetConn)
	}))
	t.Cleanup(echoServer.Close)

	host, port, err := netSplitHostPort(echoServer.Listener.Addr().String())
	if err != nil {
		t.Fatalf("split proxy addr: %v", err)
	}

	pool := x509.NewCertPool()
	pool.AddCert(tlsServer.Certificate())

	cfg := Config{
		HTTPSProbeURL:     tlsServer.URL,
		httpsTLSRoots:     pool,
		UserAgent:         "proxy-pulse-test",
		EnrichmentTimeout: 5 * time.Second,
	}

	if !probeHTTPS(context.Background(), cfg, host, port) {
		t.Fatalf("expected CONNECT-capable proxy to pass https probe")
	}

	cfg.HTTPSProbeURL = ""
	if probeHTTPS(context.Background(), cfg, host, port) {
		t.Fatalf("expected no probe without configured url")
	}
}

func relay(a net.Conn, b net.Conn) {
	done := make(chan struct{}, 2)
	go func() {
		defer func() { done <- struct{}{} }()
		buf := make([]byte, 4096)
		for {
			n, err := a.Read(buf)
			if n > 0 {
				if _, werr := b.Write(buf[:n]); werr != nil {
					return
				}
			}
			if err != nil {
				return
			}
		}
	}()
	go func() {
		defer func() { done <- struct{}{} }()
		buf := make([]byte, 4096)
		for {
			n, err := b.Read(buf)
			if n > 0 {
				if _, werr := a.Write(buf[:n]); werr != nil {
					return
				}
			}
			if err != nil {
				return
			}
		}
	}()
	<-done
}
