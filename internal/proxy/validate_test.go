package proxy

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestValidateCandidateHTTPEnrichesMetadata(t *testing.T) {
	validator, candidate := newHTTPValidatorFixture(t, httpProxyBehavior{
		primaryIP:   "8.8.8.8",
		secondaryIP: "8.8.8.8",
		anonOrigin:  "8.8.8.8",
		anonHeaders: map[string]string{
			"Accept": "*/*",
		},
	}, "9.9.9.9")

	proxy, ok, err := validator.ValidateCandidate(context.Background(), candidate)
	if err != nil {
		t.Fatalf("validate candidate: %v", err)
	}
	if !ok {
		t.Fatalf("expected proxy validation success")
	}
	if proxy.Protocol != ProtocolHTTP {
		t.Fatalf("expected http protocol, got %s", proxy.Protocol)
	}
	if proxy.ExitIP != "8.8.8.8" {
		t.Fatalf("expected exit ip 8.8.8.8, got %s", proxy.ExitIP)
	}
	if proxy.CountryCode != "US" || proxy.CountryName != "United States" {
		t.Fatalf("expected US country metadata, got %s / %s", proxy.CountryCode, proxy.CountryName)
	}
	if proxy.Anonymity != AnonymityElite {
		t.Fatalf("expected elite anonymity, got %s", proxy.Anonymity)
	}
	if proxy.LastCheckedAt == "" {
		t.Fatalf("expected last checked timestamp")
	}
}

func TestValidateCandidateHTTPClassifiesAnonymity(t *testing.T) {
	testCases := []struct {
		name      string
		behavior  httpProxyBehavior
		wantLevel AnonymityLevel
	}{
		{
			name: "transparent",
			behavior: httpProxyBehavior{
				primaryIP:   "8.8.8.8",
				secondaryIP: "8.8.8.8",
				anonOrigin:  "8.8.8.8",
				anonHeaders: map[string]string{
					"X-Forwarded-For": "9.9.9.9",
				},
			},
			wantLevel: AnonymityTransparent,
		},
		{
			name: "anonymous",
			behavior: httpProxyBehavior{
				primaryIP:   "8.8.8.8",
				secondaryIP: "8.8.8.8",
				anonOrigin:  "8.8.8.8",
				anonHeaders: map[string]string{
					"Via": "1.1 proxy",
				},
			},
			wantLevel: AnonymityAnonymous,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			validator, candidate := newHTTPValidatorFixture(t, tc.behavior, "9.9.9.9")

			proxy, ok, err := validator.ValidateCandidate(context.Background(), candidate)
			if err != nil {
				t.Fatalf("validate candidate: %v", err)
			}
			if !ok {
				t.Fatalf("expected proxy validation success")
			}
			if proxy.Anonymity != tc.wantLevel {
				t.Fatalf("expected anonymity %s, got %s", tc.wantLevel, proxy.Anonymity)
			}
		})
	}
}

func TestValidateCandidateHTTPRejectsSecondaryMismatch(t *testing.T) {
	validator, candidate := newHTTPValidatorFixture(t, httpProxyBehavior{
		primaryIP:   "8.8.8.8",
		secondaryIP: "1.1.1.1",
		anonOrigin:  "8.8.8.8",
	}, "9.9.9.9")

	_, ok, err := validator.ValidateCandidate(context.Background(), candidate)
	if err != nil {
		t.Fatalf("validate candidate: %v", err)
	}
	if ok {
		t.Fatalf("expected mismatched secondary check to fail validation")
	}
}

func TestValidateCandidateHTTPRejectsDirectIPMatch(t *testing.T) {
	validator, candidate := newHTTPValidatorFixture(t, httpProxyBehavior{
		primaryIP:   "8.8.8.8",
		secondaryIP: "8.8.8.8",
		anonOrigin:  "8.8.8.8",
	}, "8.8.8.8")

	_, ok, err := validator.ValidateCandidate(context.Background(), candidate)
	if err != nil {
		t.Fatalf("validate candidate: %v", err)
	}
	if ok {
		t.Fatalf("expected direct ip match to fail validation")
	}
}

type httpProxyBehavior struct {
	primaryIP   string
	secondaryIP string
	anonOrigin  string
	anonHeaders map[string]string
}

func newHTTPValidatorFixture(t *testing.T, behavior httpProxyBehavior, directIP string) (*Validator, Candidate) {
	t.Helper()

	directServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(directIP))
	}))
	t.Cleanup(directServer.Close)

	geoServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/geo/") {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status":      "success",
			"country":     "United States",
			"countryCode": "US",
		})
	}))
	t.Cleanup(geoServer.Close)

	proxyServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("expected GET request, got %s", r.Method)
		}
		if !strings.HasPrefix(r.RequestURI, "http://example.test/") {
			t.Fatalf("expected absolute-form proxy request uri, got %s", r.RequestURI)
		}

		switch r.URL.Path {
		case "/primary":
			_, _ = w.Write([]byte(behavior.primaryIP))
		case "/secondary":
			_, _ = w.Write([]byte(behavior.secondaryIP))
		case "/anon":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"origin":  behavior.anonOrigin,
				"headers": behavior.anonHeaders,
			})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(proxyServer.Close)

	cfg := Config{
		ValidationTimeout:  2 * time.Second,
		UserAgent:          "proxy-pulse-test",
		Concurrency:        1,
		IPEchoURL:          "http://example.test/primary",
		IPEchoURLPrimary:   "http://example.test/primary",
		IPEchoURLSecondary: "http://example.test/secondary",
		DirectIPEchoURL:    directServer.URL,
		GEOIPURLTemplate:   geoServer.URL + "/geo/%s",
		AnonCheckURL:       "http://example.test/anon",
	}

	counter := &RequestCounter{}
	validator := NewValidator(cfg, counter)

	address := proxyServer.Listener.Addr().String()
	host, port, err := netSplitHostPort(address)
	if err != nil {
		t.Fatalf("split host port: %v", err)
	}

	return validator, Candidate{
		Host:          host,
		Port:          port,
		HintProtocols: []Protocol{ProtocolHTTP},
	}
}
