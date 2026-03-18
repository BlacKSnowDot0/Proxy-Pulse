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
		if r.Method != http.MethodGet {
			t.Fatalf("expected GET request, got %s", r.Method)
		}
		if r.RequestURI != "http://example.test/ip" {
			t.Fatalf("expected absolute-form proxy request URI, got %s", r.RequestURI)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("8.8.8.8"))
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

func TestValidateCandidateHTTPRejectsOriginServerBody(t *testing.T) {
	originServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("<html>cdnjs</html>"))
	}))
	defer originServer.Close()

	cfg := Config{
		ValidationTimeout: 2 * time.Second,
		IPEchoURL:         "http://example.test/ip",
		UserAgent:         "proxy-pulse-test",
		Concurrency:       1,
	}

	counter := &RequestCounter{}
	validator := NewValidator(cfg, counter)

	address := originServer.Listener.Addr().String()
	host, port, err := netSplitHostPort(address)
	if err != nil {
		t.Fatalf("split host port: %v", err)
	}

	_, ok, err := validator.ValidateCandidate(context.Background(), Candidate{
		Host:          host,
		Port:          port,
		HintProtocols: []Protocol{ProtocolHTTP},
	})
	if err != nil {
		t.Fatalf("validate candidate: %v", err)
	}
	if ok {
		t.Fatalf("expected origin server response body to fail proxy validation")
	}
}

func TestValidateCandidateHonorsPortPreference(t *testing.T) {
	cfg := Config{
		ValidationTimeout: 200 * time.Millisecond,
		IPEchoURL:         "http://example.test/ip",
		UserAgent:         "proxy-pulse-test",
		Concurrency:       1,
	}

	counter := &RequestCounter{}
	validator := NewValidator(cfg, counter)

	_, ok, err := validator.ValidateCandidate(context.Background(), Candidate{
		Host: "203.0.113.1",
		Port: 1080,
	})
	if err != nil {
		t.Fatalf("validate candidate: %v", err)
	}
	if ok {
		t.Fatalf("expected unroutable test candidate to fail validation")
	}
}
