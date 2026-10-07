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

func TestProxyRevokesUpgradeConnectionsOnDisableReconfigureAndClose(t *testing.T) {
	for _, action := range []string{"disable", "listener-change", "route-removal", "close"} {
		t.Run(action, func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				conn, rw, err := w.(http.Hijacker).Hijack()
				if err != nil {
					return
				}
				defer conn.Close()
				_, _ = rw.WriteString("HTTP/1.1 101 Switching Protocols\r\nConnection: Upgrade\r\nUpgrade: audit\r\n\r\n")
				_ = rw.Flush()
				for {
					line, err := rw.ReadString('\n')
					if err != nil {
						return
					}
					_, _ = rw.WriteString("echo:" + line)
					_ = rw.Flush()
				}
			}))
			defer upstream.Close()
			_, text, _ := net.SplitHostPort(strings.TrimPrefix(upstream.URL, "http://"))
			port, _ := strconv.Atoi(text)
			m := New(func(string, int) (int, bool) { return port, true })
			defer m.Close()
			projects := []model.Project{{Services: []model.Service{{ID: "fixture", Route: &model.Route{Hostname: "fixture.localhost", TargetPort: port}}}}}
			settings := model.ProxySettings{Enabled: true, ListenHost: "127.0.0.1", HTTPPort: 0}
			if err := m.Configure(settings, projects); err != nil {
				t.Fatal(err)
			}
			generation := m.connections
			conn, err := net.DialTimeout("tcp", m.listener.Addr().String(), time.Second)
			if err != nil {
				t.Fatal(err)
			}
			defer conn.Close()
			_ = conn.SetDeadline(time.Now().Add(2 * time.Second))
			_, _ = fmt.Fprint(conn, "GET / HTTP/1.1\r\nHost: fixture.localhost\r\nConnection: Upgrade\r\nUpgrade: audit\r\n\r\n")
			reader := bufio.NewReader(conn)
			status, err := reader.ReadString('\n')
			if err != nil || !strings.Contains(status, "101") {
				t.Fatalf("upgrade=%q %v", status, err)
			}
			for {
				line, err := reader.ReadString('\n')
				if err != nil {
					t.Fatal(err)
				}
				if line == "\r\n" {
					break
				}
			}
			switch action {
			case "disable":
				settings.Enabled = false
				err = m.Configure(settings, projects)
			case "listener-change":
				reservation, reserveErr := net.Listen("tcp", "127.0.0.1:0")
				if reserveErr != nil {
					t.Fatal(reserveErr)
				}
				settings.HTTPPort = reservation.Addr().(*net.TCPAddr).Port
				reservation.Close()
				err = m.Configure(settings, projects)
			case "route-removal":
				err = m.Configure(settings, nil)
			default:
				err = m.Close()
			}
			if err != nil {
				t.Fatal(err)
			}
			_, _ = fmt.Fprint(conn, "after-disable\n")
			if line, err := reader.ReadString('\n'); err == nil {
				t.Fatalf("revoked tunnel still forwards %q", line)
			}
			deadline := time.Now().Add(time.Second)
			for len(m.requests) != 0 && time.Now().Before(deadline) {
				time.Sleep(time.Millisecond)
			}
			if len(m.requests) != 0 {
				t.Fatal("upgrade permit leaked")
			}
			generation.mu.Lock()
			count := len(generation.connections)
			generation.mu.Unlock()
			if count != 0 {
				t.Fatalf("tracked connections leaked: %d", count)
			}
		})
	}
}

func TestProxyDisableCancelsOrdinaryActiveStream(t *testing.T) {
	cancelled := make(chan struct{})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, "before\n")
		w.(http.Flusher).Flush()
		<-r.Context().Done()
		close(cancelled)
	}))
	defer upstream.Close()
	_, text, _ := net.SplitHostPort(strings.TrimPrefix(upstream.URL, "http://"))
	port, _ := strconv.Atoi(text)
	m := New(func(string, int) (int, bool) { return port, true })
	defer m.Close()
	settings := model.ProxySettings{Enabled: true, ListenHost: "127.0.0.1"}
	projects := []model.Project{{Services: []model.Service{{ID: "fixture", Route: &model.Route{Hostname: "fixture.localhost", TargetPort: port}}}}}
	if err := m.Configure(settings, projects); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	request, _ := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+m.listener.Addr().String(), nil)
	request.Host = "fixture.localhost"
	client := &http.Client{Transport: &http.Transport{}}
	defer client.CloseIdleConnections()
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	reader := bufio.NewReader(response.Body)
	if line, err := reader.ReadString('\n'); err != nil || line != "before\n" {
		t.Fatalf("stream: %q %v", line, err)
	}
	settings.Enabled = false
	if err := m.Configure(settings, projects); err != nil {
		t.Fatal(err)
	}
	if _, err := io.ReadAll(reader); err == nil {
		t.Fatal("active response was not hard-closed")
	}
	select {
	case <-cancelled:
	case <-time.After(time.Second):
		t.Fatal("upstream stream was not cancelled")
	}
}
