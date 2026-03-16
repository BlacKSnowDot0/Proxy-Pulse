package proxy

import (
	"bufio"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type Validator struct {
	cfg     Config
	counter *RequestCounter
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
	var checked atomic.Int64
	var errorCount atomic.Int64

	worker := func() {
		defer wg.Done()
		for candidate := range jobs {
			checked.Add(1)
			proxy, ok, err := v.ValidateCandidate(ctx, candidate)
			if err != nil {
				errorCount.Add(1)
				continue
			}
			if ok {
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

	seen := make(map[string]Proxy)
	for proxy := range results {
		key := proxy.URI()
		if existing, ok := seen[key]; ok {
			existing.Sources = mergeSources(existing.Sources, proxy.Sources)
			seen[key] = existing
			continue
		}
		proxy.Sources = mergeSources(nil, proxy.Sources)
		seen[key] = proxy
	}

	out := make([]Proxy, 0, len(seen))
	for _, proxy := range seen {
		out = append(out, proxy)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Protocol == out[j].Protocol {
			if out[i].Host == out[j].Host {
				return out[i].Port < out[j].Port
			}
			return out[i].Host < out[j].Host
		}
		return out[i].Protocol < out[j].Protocol
	})

	return out, int(checked.Load()), int(errorCount.Load())
}

func (v *Validator) ValidateCandidate(ctx context.Context, candidate Candidate) (Proxy, bool, error) {
	protocols := protocolList(candidate.HintProtocols)
	for _, protocol := range protocols {
		ok, err := v.validateProtocol(ctx, protocol, candidate)
		if err != nil {
			return Proxy{}, false, err
		}
		if ok {
			return Proxy{
				Protocol: protocol,
				Host:     candidate.Host,
				Port:     candidate.Port,
				Sources:  candidate.Sources,
			}, true, nil
		}
	}
	return Proxy{}, false, nil
}

func (v *Validator) validateProtocol(ctx context.Context, protocol Protocol, candidate Candidate) (bool, error) {
	switch protocol {
	case ProtocolHTTP:
		return v.validateHTTP(ctx, candidate)
	case ProtocolSOCKS4:
		return v.validateSOCKS(ctx, candidate, ProtocolSOCKS4)
	case ProtocolSOCKS5:
		return v.validateSOCKS(ctx, candidate, ProtocolSOCKS5)
	default:
		return false, nil
	}
}

func (v *Validator) validateHTTP(ctx context.Context, candidate Candidate) (bool, error) {
	echoURL, err := url.Parse(v.cfg.IPEchoURL)
	if err != nil {
		return false, fmt.Errorf("parse IP_ECHO_URL: %w", err)
	}

	proxyURL, err := url.Parse("http://" + candidate.Address())
	if err != nil {
		return false, nil
	}

	transport := &http.Transport{
		Proxy:                 http.ProxyURL(proxyURL),
		DialContext:           (&net.Dialer{Timeout: v.cfg.ValidationTimeout}).DialContext,
		TLSHandshakeTimeout:   v.cfg.ValidationTimeout,
		ResponseHeaderTimeout: v.cfg.ValidationTimeout,
		ForceAttemptHTTP2:     false,
	}
	defer transport.CloseIdleConnections()

	client := &http.Client{
		Timeout:   v.cfg.ValidationTimeout,
		Transport: transport,
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, echoURL.String(), nil)
	if err != nil {
		return false, err
	}
	req.Header.Set("User-Agent", v.cfg.UserAgent)

	v.counter.Inc()
	resp, err := client.Do(req)
	if err != nil {
		return false, nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return false, nil
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 128))
	if err != nil {
		return false, nil
	}
	return strings.TrimSpace(string(body)) != "", nil
}

func (v *Validator) validateSOCKS(ctx context.Context, candidate Candidate, protocol Protocol) (bool, error) {
	echoURL, err := url.Parse(v.cfg.IPEchoURL)
	if err != nil {
		return false, fmt.Errorf("parse IP_ECHO_URL: %w", err)
	}
	if echoURL.Scheme != "http" {
		return false, fmt.Errorf("socks validation requires http IP_ECHO_URL, got %s", echoURL.Scheme)
	}

	requestCtx, cancel := context.WithTimeout(ctx, v.cfg.ValidationTimeout)
	defer cancel()

	conn, err := v.openSOCKSTunnel(requestCtx, candidate.Address(), echoURL.Hostname(), echoURL.Port(), protocol)
	if err != nil {
		return false, nil
	}
	defer conn.Close()

	if err := conn.SetDeadline(time.Now().Add(v.cfg.ValidationTimeout)); err != nil {
		return false, nil
	}

	path := echoURL.RequestURI()
	if path == "" {
		path = "/"
	}
	if _, err := fmt.Fprintf(conn, "GET %s HTTP/1.1\r\nHost: %s\r\nUser-Agent: %s\r\nConnection: close\r\n\r\n", path, echoURL.Host, v.cfg.UserAgent); err != nil {
		return false, nil
	}
	v.counter.Inc()

	req, _ := http.NewRequest(http.MethodGet, echoURL.String(), nil)
	resp, err := http.ReadResponse(bufio.NewReader(conn), req)
	if err != nil {
		return false, nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return false, nil
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 128))
	if err != nil {
		return false, nil
	}
	return strings.TrimSpace(string(body)) != "", nil
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
