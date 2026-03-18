package proxy

import (
	"bufio"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type Validator struct {
	cfg     Config
	counter *RequestCounter

	directIPOnce sync.Once
	directIP     string
	directIPErr  error
}

type validationAttempt struct {
	proxy  Proxy
	reason string
}

type proxyInspection struct {
	Origin  string
	Headers map[string]string
}

func NewValidator(cfg Config, counter *RequestCounter) *Validator {
	return &Validator{cfg: cfg, counter: counter}
}

func (v *Validator) ValidateAll(ctx context.Context, candidates []Candidate) ([]Proxy, int, int) {
	if len(candidates) == 0 {
		return nil, 0, 0
	}

	workers := v.cfg.Concurrency
	if workers > len(candidates) {
		workers = len(candidates)
	}
	if workers < 1 {
		workers = 1
	}

	jobs := make(chan Candidate)
	results := make(chan Proxy, len(candidates))
	var wg sync.WaitGroup
	var startedCount atomic.Int64
	var completedCount atomic.Int64
	var errorCount atomic.Int64
	var validatedCount atomic.Int64

	if interval := v.cfg.ValidationLogInterval; interval > 0 {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		started := time.Now()
		done := make(chan struct{})
		defer close(done)

		go func() {
			for {
				select {
				case <-done:
					return
				case <-ticker.C:
					startedNow := startedCount.Load()
					completedNow := completedCount.Load()
					validNow := validatedCount.Load()
					errNow := errorCount.Load()
					inFlight := startedNow - completedNow
					rate := float64(completedNow)
					if elapsed := time.Since(started).Seconds(); elapsed > 0 {
						rate = rate / elapsed
					}
					log.Printf("validation progress: started=%d/%d completed=%d/%d inflight=%d validated=%d errors=%d rate=%.2f/s",
						startedNow, len(candidates), completedNow, len(candidates), inFlight, validNow, errNow, rate)
				}
			}
		}()
	}

	worker := func() {
		defer wg.Done()
		for candidate := range jobs {
			if ctx.Err() != nil {
				return
			}
			startedCount.Add(1)
			proxy, ok, err := v.ValidateCandidate(ctx, candidate)
			completedCount.Add(1)
			if err != nil {
				if ctx.Err() != nil {
					return
				}
				errorCount.Add(1)
				continue
			}
			if ok {
				validatedCount.Add(1)
				log.Printf("validated proxy: %s via %s exit=%s anonymity=%s country=%s",
					proxy.URI(), proxy.Protocol, proxy.ExitIP, proxy.Anonymity, proxy.CountryCode)
				results <- proxy
			}
		}
	}

	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go worker()
	}

	go func() {
		defer close(jobs)
		for _, candidate := range candidates {
			select {
			case <-ctx.Done():
				return
			case jobs <- candidate:
			}
		}
	}()

	wg.Wait()
	close(results)

	collected := make([]Proxy, 0, len(results))
	for proxy := range results {
		collected = append(collected, proxy)
	}

	return mergeProxySlice(collected), int(completedCount.Load()), int(errorCount.Load())
}

func (v *Validator) ValidateCandidate(ctx context.Context, candidate Candidate) (Proxy, bool, error) {
	protocols := preferredProtocols(candidate)
	for _, protocol := range protocols {
		attempt, err := v.validateProtocol(ctx, protocol, candidate)
		if err != nil {
			if ctx.Err() != nil {
				return Proxy{}, false, ctx.Err()
			}
			return Proxy{}, false, err
		}
		if attempt.proxy.Protocol != "" {
			return attempt.proxy, true, nil
		}
		if attempt.reason != "" {
			log.Printf("rejected proxy: %s via %s reason=%s", candidate.Address(), protocol, attempt.reason)
		}
	}
	return Proxy{}, false, nil
}

func (v *Validator) validateProtocol(ctx context.Context, protocol Protocol, candidate Candidate) (validationAttempt, error) {
	switch protocol {
	case ProtocolHTTP:
		return v.validateHTTP(ctx, candidate)
	case ProtocolSOCKS4:
		return v.validateSOCKS(ctx, candidate, ProtocolSOCKS4)
	case ProtocolSOCKS5:
		return v.validateSOCKS(ctx, candidate, ProtocolSOCKS5)
	default:
		return validationAttempt{reason: "unsupported_protocol"}, nil
	}
}

func (v *Validator) validateHTTP(ctx context.Context, candidate Candidate) (validationAttempt, error) {
	primaryURL, secondaryURL, err := v.validationEchoTargets()
	if err != nil {
		return validationAttempt{}, err
	}

	directIP, err := v.getDirectIP(ctx)
	if err != nil {
		return validationAttempt{}, err
	}

	exitIP, reason := v.verifyHTTPProxyExit(ctx, candidate, primaryURL, secondaryURL, directIP)
	if reason != "" {
		return validationAttempt{reason: reason}, nil
	}

	countryCode, countryName := v.lookupCountry(ctx, exitIP)
	attempt := validationAttempt{
		proxy: Proxy{
			Protocol:      ProtocolHTTP,
			Host:          candidate.Host,
			Port:          candidate.Port,
			Sources:       candidate.Sources,
			ExitIP:        exitIP,
			CountryCode:   countryCode,
			CountryName:   countryName,
			Anonymity:     v.classifyHTTPAnonymity(ctx, candidate, directIP),
			LastCheckedAt: time.Now().UTC().Format(time.RFC3339),
		},
	}
	return attempt, nil
}

func (v *Validator) validateSOCKS(ctx context.Context, candidate Candidate, protocol Protocol) (validationAttempt, error) {
	primaryURL, secondaryURL, err := v.validationEchoTargets()
	if err != nil {
		return validationAttempt{}, err
	}
	if primaryURL.Scheme != "http" || secondaryURL.Scheme != "http" {
		return validationAttempt{}, fmt.Errorf("socks validation requires http echo URLs")
	}

	directIP, err := v.getDirectIP(ctx)
	if err != nil {
		return validationAttempt{}, err
	}

	exitIP, reason := v.verifySOCKSProxyExit(ctx, candidate, protocol, primaryURL, secondaryURL, directIP)
	if reason != "" {
		return validationAttempt{reason: reason}, nil
	}

	countryCode, countryName := v.lookupCountry(ctx, exitIP)
	attempt := validationAttempt{
		proxy: Proxy{
			Protocol:      protocol,
			Host:          candidate.Host,
			Port:          candidate.Port,
			Sources:       candidate.Sources,
			ExitIP:        exitIP,
			CountryCode:   countryCode,
			CountryName:   countryName,
			Anonymity:     AnonymityUnknown,
			LastCheckedAt: time.Now().UTC().Format(time.RFC3339),
		},
	}
	return attempt, nil
}

func (v *Validator) verifyHTTPProxyExit(ctx context.Context, candidate Candidate, primaryURL *url.URL, secondaryURL *url.URL, directIP string) (string, string) {
	primaryIP, err := v.fetchEchoIPViaHTTPProxy(ctx, candidate, primaryURL)
	if err != nil {
		return "", "primary_probe_failed"
	}

	secondaryIP, err := v.fetchEchoIPViaHTTPProxy(ctx, candidate, secondaryURL)
	if err != nil {
		return "", "secondary_probe_failed"
	}

	if primaryIP != secondaryIP {
		return "", "exit_ip_mismatch"
	}
	if primaryIP == directIP {
		return "", "direct_ip_match"
	}
	return primaryIP, ""
}

func (v *Validator) verifySOCKSProxyExit(ctx context.Context, candidate Candidate, protocol Protocol, primaryURL *url.URL, secondaryURL *url.URL, directIP string) (string, string) {
	primaryIP, err := v.fetchEchoIPViaSOCKS(ctx, candidate, protocol, primaryURL)
	if err != nil {
		return "", "primary_probe_failed"
	}

	secondaryIP, err := v.fetchEchoIPViaSOCKS(ctx, candidate, protocol, secondaryURL)
	if err != nil {
		return "", "secondary_probe_failed"
	}

	if primaryIP != secondaryIP {
		return "", "exit_ip_mismatch"
	}
	if primaryIP == directIP {
		return "", "direct_ip_match"
	}
	return primaryIP, ""
}

func (v *Validator) validationEchoTargets() (*url.URL, *url.URL, error) {
	primaryURL, err := url.Parse(strings.TrimSpace(v.cfg.IPEchoURLPrimary))
	if err != nil {
		return nil, nil, fmt.Errorf("parse IP_ECHO_URL_PRIMARY: %w", err)
	}
	if primaryURL.Scheme == "" || primaryURL.Host == "" {
		return nil, nil, fmt.Errorf("parse IP_ECHO_URL_PRIMARY: missing scheme or host")
	}

	secondaryURL, err := url.Parse(strings.TrimSpace(v.cfg.IPEchoURLSecondary))
	if err != nil {
		return nil, nil, fmt.Errorf("parse IP_ECHO_URL_SECONDARY: %w", err)
	}
	if secondaryURL.Scheme == "" || secondaryURL.Host == "" {
		return nil, nil, fmt.Errorf("parse IP_ECHO_URL_SECONDARY: missing scheme or host")
	}

	return primaryURL, secondaryURL, nil
}

func (v *Validator) getDirectIP(ctx context.Context) (string, error) {
	v.directIPOnce.Do(func() {
		directURL, err := url.Parse(strings.TrimSpace(v.cfg.DirectIPEchoURL))
		if err != nil {
			v.directIPErr = fmt.Errorf("parse DIRECT_IP_ECHO_URL: %w", err)
			return
		}
		if directURL.Scheme == "" || directURL.Host == "" {
			v.directIPErr = fmt.Errorf("parse DIRECT_IP_ECHO_URL: missing scheme or host")
			return
		}

		v.directIP, v.directIPErr = v.fetchEchoIPDirect(ctx, directURL)
	})
	return v.directIP, v.directIPErr
}

func (v *Validator) fetchEchoIPDirect(ctx context.Context, targetURL *url.URL) (string, error) {
	client, transport := v.newHTTPClient(nil)
	defer transport.CloseIdleConnections()
	return v.fetchEchoIP(ctx, client, targetURL)
}

func (v *Validator) fetchEchoIPViaHTTPProxy(ctx context.Context, candidate Candidate, targetURL *url.URL) (string, error) {
	proxyURL, err := url.Parse("http://" + candidate.Address())
	if err != nil {
		return "", err
	}

	client, transport := v.newHTTPClient(proxyURL)
	defer transport.CloseIdleConnections()
	return v.fetchEchoIP(ctx, client, targetURL)
}

func (v *Validator) fetchEchoIP(ctx context.Context, client *http.Client, targetURL *url.URL) (string, error) {
	requestCtx, cancel := context.WithTimeout(ctx, v.cfg.ValidationTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(requestCtx, http.MethodGet, targetURL.String(), nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", v.cfg.UserAgent)

	v.counter.Inc()
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 128))
	if err != nil {
		return "", err
	}
	return parsePublicIPv4Body(body)
}

func (v *Validator) fetchEchoIPViaSOCKS(ctx context.Context, candidate Candidate, protocol Protocol, targetURL *url.URL) (string, error) {
	requestCtx, cancel := context.WithTimeout(ctx, v.cfg.ValidationTimeout)
	defer cancel()

	conn, err := v.openSOCKSTunnel(requestCtx, candidate.Address(), targetURL.Hostname(), targetURL.Port(), protocol)
	if err != nil {
		return "", err
	}
	defer conn.Close()

	if err := conn.SetDeadline(time.Now().Add(v.cfg.ValidationTimeout)); err != nil {
		return "", err
	}

	path := targetURL.RequestURI()
	if path == "" {
		path = "/"
	}
	if _, err := fmt.Fprintf(conn, "GET %s HTTP/1.1\r\nHost: %s\r\nUser-Agent: %s\r\nConnection: close\r\n\r\n", path, targetURL.Host, v.cfg.UserAgent); err != nil {
		return "", err
	}
	v.counter.Inc()

	req, _ := http.NewRequest(http.MethodGet, targetURL.String(), nil)
	resp, err := http.ReadResponse(bufio.NewReader(conn), req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 128))
	if err != nil {
		return "", err
	}
	return parsePublicIPv4Body(body)
}

func parsePublicIPv4Body(body []byte) (string, error) {
	value := strings.TrimSpace(string(body))
	if value == "" {
		return "", fmt.Errorf("empty body")
	}

	ip := net.ParseIP(value)
	if ip == nil {
		return "", fmt.Errorf("invalid ip body %q", value)
	}

	ipv4 := ip.To4()
	if ipv4 == nil || !isPublicIPv4(ipv4) {
		return "", fmt.Errorf("non-public ipv4 %q", value)
	}
	return ipv4.String(), nil
}

func (v *Validator) lookupCountry(ctx context.Context, ip string) (string, string) {
	template := strings.TrimSpace(v.cfg.GEOIPURLTemplate)
	if template == "" || strings.TrimSpace(ip) == "" {
		return "", ""
	}

	escapedIP := url.PathEscape(ip)
	target := template
	switch {
	case strings.Contains(target, "%s"):
		target = fmt.Sprintf(target, escapedIP)
	case strings.Contains(target, "{ip}"):
		target = strings.ReplaceAll(target, "{ip}", escapedIP)
	default:
		target = strings.TrimRight(target, "/") + "/" + escapedIP
	}

	targetURL, err := url.Parse(target)
	if err != nil || targetURL.Scheme == "" || targetURL.Host == "" {
		return "", ""
	}

	client, transport := v.newHTTPClient(nil)
	defer transport.CloseIdleConnections()

	requestCtx, cancel := context.WithTimeout(ctx, v.cfg.ValidationTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(requestCtx, http.MethodGet, targetURL.String(), nil)
	if err != nil {
		return "", ""
	}
	req.Header.Set("User-Agent", v.cfg.UserAgent)

	v.counter.Inc()
	resp, err := client.Do(req)
	if err != nil {
		return "", ""
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", ""
	}

	var payload struct {
		Status      string `json:"status"`
		Country     string `json:"country"`
		CountryCode string `json:"countryCode"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4096)).Decode(&payload); err != nil {
		return "", ""
	}
	if payload.Status != "" && payload.Status != "success" {
		return "", ""
	}
	return strings.ToUpper(strings.TrimSpace(payload.CountryCode)), strings.TrimSpace(payload.Country)
}

func (v *Validator) classifyHTTPAnonymity(ctx context.Context, candidate Candidate, directIP string) AnonymityLevel {
	anonURL := strings.TrimSpace(v.cfg.AnonCheckURL)
	if anonURL == "" {
		return AnonymityUnknown
	}

	targetURL, err := url.Parse(anonURL)
	if err != nil || targetURL.Scheme == "" || targetURL.Host == "" {
		return AnonymityUnknown
	}

	inspection, err := v.fetchProxyInspectionViaHTTP(ctx, candidate, targetURL)
	if err != nil {
		return AnonymityUnknown
	}

	if leaksDirectIdentity(inspection, directIP) {
		return AnonymityTransparent
	}
	if exposesProxyIdentity(inspection) {
		return AnonymityAnonymous
	}
	return AnonymityElite
}

func (v *Validator) fetchProxyInspectionViaHTTP(ctx context.Context, candidate Candidate, targetURL *url.URL) (proxyInspection, error) {
	proxyURL, err := url.Parse("http://" + candidate.Address())
	if err != nil {
		return proxyInspection{}, err
	}

	client, transport := v.newHTTPClient(proxyURL)
	defer transport.CloseIdleConnections()

	requestCtx, cancel := context.WithTimeout(ctx, v.cfg.ValidationTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(requestCtx, http.MethodGet, targetURL.String(), nil)
	if err != nil {
		return proxyInspection{}, err
	}
	req.Header.Set("User-Agent", v.cfg.UserAgent)

	v.counter.Inc()
	resp, err := client.Do(req)
	if err != nil {
		return proxyInspection{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return proxyInspection{}, fmt.Errorf("unexpected status %d", resp.StatusCode)
	}

	var payload struct {
		Origin  any            `json:"origin"`
		Headers map[string]any `json:"headers"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 16*1024)).Decode(&payload); err != nil {
		return proxyInspection{}, err
	}

	headers := make(map[string]string, len(payload.Headers))
	for key, value := range payload.Headers {
		headers[strings.ToLower(strings.TrimSpace(key))] = strings.TrimSpace(fmt.Sprint(value))
	}

	return proxyInspection{
		Origin:  strings.TrimSpace(fmt.Sprint(payload.Origin)),
		Headers: headers,
	}, nil
}

func leaksDirectIdentity(inspection proxyInspection, directIP string) bool {
	directIP = strings.TrimSpace(directIP)
	if directIP == "" {
		return false
	}

	if containsIPToken(inspection.Origin, directIP) {
		return true
	}

	for _, key := range []string{
		"x-forwarded-for",
		"x-real-ip",
		"true-client-ip",
		"client-ip",
		"cf-connecting-ip",
		"forwarded",
	} {
		if containsIPToken(inspection.Headers[key], directIP) {
			return true
		}
	}
	return false
}

func exposesProxyIdentity(inspection proxyInspection) bool {
	for _, key := range []string{
		"via",
		"forwarded",
		"x-forwarded-for",
		"x-forwarded-host",
		"x-forwarded-proto",
		"x-real-ip",
		"true-client-ip",
		"client-ip",
		"proxy-connection",
	} {
		if strings.TrimSpace(inspection.Headers[key]) != "" {
			return true
		}
	}
	return false
}

func containsIPToken(value string, target string) bool {
	value = strings.TrimSpace(value)
	target = strings.TrimSpace(target)
	if value == "" || target == "" {
		return false
	}

	for _, token := range strings.FieldsFunc(value, func(r rune) bool {
		switch r {
		case ',', ' ', ';', '"':
			return true
		default:
			return false
		}
	}) {
		token = strings.TrimPrefix(strings.TrimSpace(token), "for=")
		token = strings.Trim(token, "[]")
		if token == target {
			return true
		}
	}
	return false
}

func (v *Validator) newHTTPClient(proxyURL *url.URL) (*http.Client, *http.Transport) {
	transport := &http.Transport{
		DialContext:           (&net.Dialer{Timeout: v.cfg.ValidationTimeout}).DialContext,
		TLSHandshakeTimeout:   v.cfg.ValidationTimeout,
		ResponseHeaderTimeout: v.cfg.ValidationTimeout,
		ForceAttemptHTTP2:     false,
	}
	if proxyURL != nil {
		transport.Proxy = http.ProxyURL(proxyURL)
	}

	return &http.Client{
		Timeout:   v.cfg.ValidationTimeout,
		Transport: transport,
	}, transport
}

func (v *Validator) openSOCKSTunnel(ctx context.Context, proxyAddr string, targetHost string, targetPort string, protocol Protocol) (net.Conn, error) {
	if targetPort == "" {
		targetPort = "80"
	}

	dialer := &net.Dialer{Timeout: v.cfg.ValidationTimeout}
	conn, err := dialer.DialContext(ctx, "tcp", proxyAddr)
	if err != nil {
		return nil, err
	}
	if err := conn.SetDeadline(time.Now().Add(v.cfg.ValidationTimeout)); err != nil {
		_ = conn.Close()
		return nil, err
	}

	success := false
	defer func() {
		if !success {
			_ = conn.Close()
		}
	}()

	switch protocol {
	case ProtocolSOCKS4:
		err = handshakeSOCKS4(conn, targetHost, targetPort)
	case ProtocolSOCKS5:
		err = handshakeSOCKS5(conn, targetHost, targetPort)
	default:
		err = fmt.Errorf("unsupported protocol %s", protocol)
	}
	if err != nil {
		return nil, err
	}

	success = true
	return conn, nil
}

func handshakeSOCKS4(conn net.Conn, host string, port string) error {
	portNumber, err := net.LookupPort("tcp", port)
	if err != nil {
		return err
	}

	packet := []byte{0x04, 0x01, 0x00, 0x00}
	binary.BigEndian.PutUint16(packet[2:4], uint16(portNumber))

	ip := net.ParseIP(host)
	if ipv4 := ip.To4(); ipv4 != nil {
		packet = append(packet, ipv4...)
		packet = append(packet, 0x00)
	} else {
		packet = append(packet, 0x00, 0x00, 0x00, 0x01, 0x00)
		packet = append(packet, host...)
		packet = append(packet, 0x00)
	}

	if _, err := conn.Write(packet); err != nil {
		return err
	}

	reply := make([]byte, 8)
	if _, err := io.ReadFull(conn, reply); err != nil {
		return err
	}
	if reply[1] != 0x5A {
		return fmt.Errorf("socks4 connect failed with code %d", reply[1])
	}
	return nil
}

func handshakeSOCKS5(conn net.Conn, host string, port string) error {
	if _, err := conn.Write([]byte{0x05, 0x01, 0x00}); err != nil {
		return err
	}
	reply := make([]byte, 2)
	if _, err := io.ReadFull(conn, reply); err != nil {
		return err
	}
	if reply[0] != 0x05 || reply[1] != 0x00 {
		return fmt.Errorf("socks5 auth negotiation failed")
	}

	portNumber, err := net.LookupPort("tcp", port)
	if err != nil {
		return err
	}

	request := []byte{0x05, 0x01, 0x00}
	if ip := net.ParseIP(host); ip != nil {
		if ipv4 := ip.To4(); ipv4 != nil {
			request = append(request, 0x01)
			request = append(request, ipv4...)
		} else {
			return fmt.Errorf("ipv6 targets are not supported")
		}
	} else {
		request = append(request, 0x03, byte(len(host)))
		request = append(request, host...)
	}

	request = append(request, 0x00, 0x00)
	binary.BigEndian.PutUint16(request[len(request)-2:], uint16(portNumber))

	if _, err := conn.Write(request); err != nil {
		return err
	}

	header := make([]byte, 4)
	if _, err := io.ReadFull(conn, header); err != nil {
		return err
	}
	if header[1] != 0x00 {
		return fmt.Errorf("socks5 connect failed with code %d", header[1])
	}

	var skip int
	switch header[3] {
	case 0x01:
		skip = 4
	case 0x03:
		length := make([]byte, 1)
		if _, err := io.ReadFull(conn, length); err != nil {
			return err
		}
		skip = int(length[0])
	case 0x04:
		skip = 16
	default:
		return fmt.Errorf("unsupported socks5 atyp %d", header[3])
	}

	if skip > 0 {
		if _, err := io.CopyN(io.Discard, conn, int64(skip)); err != nil {
			return err
		}
	}
	if _, err := io.CopyN(io.Discard, conn, 2); err != nil {
		return err
	}
	return nil
}
