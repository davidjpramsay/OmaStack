package proxy

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"omastack/internal/model"
)

func testRoute(t *testing.T, m *Manager, host, service, upstream string) {
	t.Helper()
	_, portText, err := net.SplitHostPort(strings.TrimPrefix(upstream, "http://"))
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		t.Fatal(err)
	}
	m.routes[host] = routeTarget{serviceID: service, port: port}
}

func proxyGet(ctx context.Context, endpoint, host string) (*http.Response, error) {
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	req.Host = host
	return http.DefaultClient.Do(req)
}

func TestProxyTimesOutStalledHeadersAndBodies(t *testing.T) {
	for _, headers := range []bool{false, true} {
		t.Run(fmt.Sprint("headers=", headers), func(t *testing.T) {
			up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if headers {
					w.Header().Set("Content-Type", "text/event-stream")
					w.Header().Set("Content-Length", "100")
					w.WriteHeader(200)
					w.(http.Flusher).Flush()
				}
				<-r.Context().Done()
			}))
			defer up.Close()
			m := New(nil)
			m.transport = upstreamTransport(100*time.Millisecond, 150*time.Millisecond)
			defer m.Close()
			testRoute(t, m, "stall.localhost", "stall", up.URL)
			server := httptest.NewServer(m)
			defer server.Close()
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			response, err := proxyGet(ctx, server.URL, "stall.localhost")
			if err != nil {
				t.Fatal(err)
			}
			_, readErr := io.ReadAll(response.Body)
			response.Body.Close()
			if headers && readErr == nil {
				t.Fatal("stalled body did not abort")
			}
			if !headers && response.StatusCode != 502 {
				t.Fatalf("status=%d", response.StatusCode)
			}
			if ctx.Err() != nil {
				t.Fatal("only outer test deadline released request")
			}
			deadline := time.Now().Add(time.Second)
			for len(m.requests) != 0 && time.Now().Before(deadline) {
				time.Sleep(time.Millisecond)
			}
			if len(m.requests) != 0 {
				t.Fatal("permit not released")
			}
		})
	}
}

func TestProxyIsolatesServiceCapacityAndReleasesOnCancellation(t *testing.T) {
	entered := make(chan struct{}, 16)
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		entered <- struct{}{}
		<-r.Context().Done()
	}))
	defer up.Close()
	healthy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, "healthy") }))
	defer healthy.Close()
	m := New(nil)
	defer m.Close()
	testRoute(t, m, "stall.localhost", "stall", up.URL)
	testRoute(t, m, "alias.localhost", "stall", up.URL)
	testRoute(t, m, "healthy.localhost", "healthy", healthy.URL)
	server := httptest.NewServer(m)
	defer server.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{}, 16)
	for i := 0; i < 16; i++ {
		go func() {
			defer func() { done <- struct{}{} }()
			response, _ := proxyGet(ctx, server.URL, "stall.localhost")
			if response != nil {
				response.Body.Close()
			}
		}()
	}
	for i := 0; i < 16; i++ {
		select {
		case <-entered:
		case <-time.After(3 * time.Second):
			t.Fatal("requests not admitted")
		}
	}
	for _, host := range []string{"stall.localhost", "alias.localhost", "healthy.localhost"} {
		limited, c := context.WithTimeout(context.Background(), time.Second)
		response, err := proxyGet(limited, server.URL, host)
		if err != nil {
			c()
			t.Fatal(err)
		}
		body, _ := io.ReadAll(response.Body)
		response.Body.Close()
		c()
		if host == "healthy.localhost" {
			if response.StatusCode != 200 || string(body) != "healthy" {
				t.Fatalf("healthy route blocked: %d", response.StatusCode)
			}
		} else if response.StatusCode != 503 {
			t.Fatalf("quota bypass on %s: %d", host, response.StatusCode)
		}
	}
	cancel()
	for i := 0; i < 16; i++ {
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatal("cancel did not finish clients")
		}
	}
	deadline := time.Now().Add(time.Second)
	for len(m.requests) != 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if len(m.requests) != 0 {
		t.Fatal("cancel leaked permits")
	}
}

func TestProxyPreservesActiveStreamingAndUpgrade(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Upgrade") == "echo" {
			conn, rw, err := w.(http.Hijacker).Hijack()
			if err != nil {
				return
			}
			defer conn.Close()
			_, _ = rw.WriteString("HTTP/1.1 101 Switching Protocols\r\nConnection: Upgrade\r\nUpgrade: echo\r\n\r\n")
			_ = rw.Flush()
			for {
				line, err := rw.ReadString('\n')
				if err != nil {
					return
				}
					if line != "silent\n" {
						_, _ = io.WriteString(conn, line)
					}
				}
		}
		for i := 0; i < 10; i++ {
			_, _ = io.WriteString(w, "chunk\n")
			w.(http.Flusher).Flush()
			time.Sleep(30 * time.Millisecond)
		}
	}))
	defer up.Close()
	m := New(nil)
	m.transport = upstreamTransport(time.Second, 200*time.Millisecond)
	defer m.Close()
	testRoute(t, m, "stream.localhost", "stream", up.URL)
	server := httptest.NewServer(m)
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	response, err := proxyGet(ctx, server.URL, "stream.localhost")
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(response.Body)
	response.Body.Close()
	if err != nil || string(body) != strings.Repeat("chunk\n", 10) {
		t.Fatalf("stream: %q %v", body, err)
	}
	conn, err := net.Dial("tcp", strings.TrimPrefix(server.URL, "http://"))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(2 * time.Second))
	_, _ = io.WriteString(conn, "GET / HTTP/1.1\r\nHost: stream.localhost\r\nConnection: Upgrade\r\nUpgrade: echo\r\n\r\n")
	reader := bufio.NewReader(conn)
	upgrade, err := http.ReadResponse(reader, nil)
	if err != nil {
		t.Fatal(err)
	}
	if upgrade.StatusCode != 101 {
		t.Fatalf("upgrade=%d", upgrade.StatusCode)
	}
	for i := 0; i < 10; i++ {
		_, _ = io.WriteString(conn, "ping\n")
		line, err := reader.ReadString('\n')
		if err != nil || line != "ping\n" {
			t.Fatalf("upgrade echo: %q %v", line, err)
		}
		time.Sleep(30 * time.Millisecond)
	}
	// Silent upgraded connections are also bounded.
	// One-way traffic is activity too, even when the peer sends no replies.
	for i := 0; i < 10; i++ {
		_, _ = io.WriteString(conn, "silent\n")
		time.Sleep(30 * time.Millisecond)
	}
	_, _ = io.WriteString(conn, "ping\n")
	if line, err := reader.ReadString('\n'); err != nil || line != "ping\n" {
		t.Fatalf("one-way active stream: %q %v", line, err)
	}
	if _, err := reader.ReadByte(); err == nil {
		t.Fatal("silent upgrade remained open")
	}
}

func TestRouteURLsAndActivityReflectListener(t *testing.T) {
	m := New(nil)
	projects := []model.Project{{Services: []model.Service{{ID: "service", Route: &model.Route{Hostname: " APP.localhost. ", TargetPort: 3000}}}}}
	if err := m.Configure(model.ProxySettings{Enabled: false, ListenHost: "127.0.0.1", HTTPPort: 8088}, projects); err != nil {
		t.Fatal(err)
	}
	routes := m.Routes()
	if len(routes) != 1 || routes[0].URL != "http://app.localhost:8088" || routes[0].Active || routes[0].Error == "" {
		t.Fatalf("disabled routes=%+v", routes)
	}
	projects[0].Services = append(projects[0].Services, model.Service{ID: "other", Route: &model.Route{Hostname: "app.localhost"}})
	if err := m.Configure(model.ProxySettings{}, projects); err == nil {
		t.Fatal("equivalent duplicate route accepted")
	}
}

func TestProxyRejectsUnknownHostAndStoppedTarget(t *testing.T) {
	manager := New(func(id string, configured int) (int, bool) { return 0, false })
	manager.routes["api.app.test"] = routeTarget{serviceID: "service", port: 80}
	unknown := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "http://unknown.example/", nil)
	request.Host = "unknown.example"
	manager.ServeHTTP(unknown, request)
	if unknown.Code != http.StatusMisdirectedRequest {
		t.Fatalf("unknown host status=%d", unknown.Code)
	}
	known := httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "http://api.app.test/", nil)
	request.Host = "api.app.test"
	manager.ServeHTTP(known, request)
	if known.Code != http.StatusBadGateway {
		t.Fatalf("stopped target status=%d", known.Code)
	}
}

func TestProxyReplacesUntrustedForwardingHeaders(t *testing.T) {
	headers := make(http.Header)
	for _, name := range []string{"Forwarded", "X-Forwarded-For", "X-Real-IP", "X-Forwarded-Port", "X-Forwarded-Server", "X-Forwarded-Client-Cert", "X-Original-URL", "X-Original-Method"} {
		headers.Set(name, "attacker-controlled")
	}
	sanitizeForwardingHeaders(headers, "api.app.test")
	for _, name := range []string{"Forwarded", "X-Forwarded-For", "X-Real-IP", "X-Forwarded-Port", "X-Forwarded-Server", "X-Forwarded-Client-Cert", "X-Original-URL", "X-Original-Method"} {
		if headers.Get(name) != "" {
			t.Fatalf("untrusted %s retained", name)
		}
	}
	if headers.Get("X-Forwarded-Host") != "api.app.test" || headers.Get("X-Forwarded-Proto") != "http" {
		t.Fatalf("canonical forwarding metadata missing: %#v", headers)
	}
}
