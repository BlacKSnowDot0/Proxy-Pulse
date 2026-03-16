package proxy

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestValidateCandidateHTTP(t *testing.T) {
	proxyServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("203.0.113.10"))
	}))
	defer proxyServer.Close()

	cfg := Config{
		ValidationTimeout: 2 * time.Second,
		IPEchoURL:         "http://example.test/ip",
		UserAgent:         "proxy-pulse-test",
		Concurrency:       1,
	}

	counter := &RequestCounter{}
	validator := NewValidator(cfg, counter)

	address := proxyServer.Listener.Addr().String()
	host, port, err := netSplitHostPort(address)
	if err != nil {
		t.Fatalf("split host port: %v", err)
	}

	proxy, ok, err := validator.ValidateCandidate(context.Background(), Candidate{
		Host:          host,
		Port:          port,
		HintProtocols: []Protocol{ProtocolHTTP},
	})
	if err != nil {
		t.Fatalf("validate candidate: %v", err)
	}
	if !ok {
		t.Fatalf("expected proxy validation success")
	}
	if proxy.Protocol != ProtocolHTTP {
		t.Fatalf("expected http protocol, got %s", proxy.Protocol)
	}
}
