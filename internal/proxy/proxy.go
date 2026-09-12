package proxy

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"omastack/internal/model"
	"omastack/internal/validate"
)

type PortResolver func(serviceID string, configured int) (int, bool)

type routeTarget struct {
	serviceID string
	port      int
}

type Manager struct {
	mu        sync.RWMutex
	server    *http.Server
	listener  net.Listener
	routes    map[string]routeTarget
	resolve   PortResolver
	address   string
	lastError string
	requests  chan struct{}
	active    map[string]int
	transport *http.Transport
	port      int
}

func New(resolve PortResolver) *Manager {
	return &Manager{routes: map[string]routeTarget{}, resolve: resolve, requests: make(chan struct{}, 64),
		active: map[string]int{}, transport: upstreamTransport(30*time.Second, 60*time.Second)}
}

// Refresh deadlines for each I/O, including upgraded connections. Active
// streams can continue; stalled headers, bodies and websocket peers cannot
// retain a route slot indefinitely. No environment proxy may redirect traffic.
func upstreamTransport(headerTimeout, idleTimeout time.Duration) *http.Transport {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	transport.ResponseHeaderTimeout = headerTimeout
	transport.MaxResponseHeaderBytes = 32 << 10
	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		conn, err := (&net.Dialer{Timeout: 5 * time.Second, KeepAlive: 30 * time.Second}).DialContext(ctx, network, address)
		if err != nil {
			return nil, err
		}
		return &idleConn{Conn: conn, timeout: idleTimeout}, nil
	}
	return transport
}

type idleConn struct {
	net.Conn
	timeout time.Duration
}

func (c *idleConn) Read(p []byte) (int, error) {
	if err := c.Conn.SetDeadline(time.Now().Add(c.timeout)); err != nil {
		return 0, err
	}
	return c.Conn.Read(p)
}

func (c *idleConn) Write(p []byte) (int, error) {
	if err := c.Conn.SetDeadline(time.Now().Add(c.timeout)); err != nil {
		return 0, err
	}
	return c.Conn.Write(p)
}

func (m *Manager) Configure(settings model.ProxySettings, projects []model.Project) error {
	routes := map[string]routeTarget{}
	for _, project := range projects {
		for _, service := range project.Services {
			if service.Route == nil {
				continue
			}
			host := validate.CanonicalHostname(service.Route.Hostname)
			if !validate.LocalHostname(host) {
				return fmt.Errorf("invalid route hostname %q", host)
			}
			if _, exists := routes[host]; exists {
				return fmt.Errorf("duplicate route hostname %q", host)
			}
			routes[host] = routeTarget{serviceID: service.ID, port: service.Route.TargetPort}
		}
	}
	m.mu.Lock()
	m.routes = routes
	m.port = settings.HTTPPort
	m.mu.Unlock()
	if !settings.Enabled {
		return m.Close()
	}
	address := net.JoinHostPort(settings.ListenHost, strconv.Itoa(settings.HTTPPort))
	m.mu.RLock()
	same := m.listener != nil && m.address == address
	m.mu.RUnlock()
	if same {
		return nil
	}
	_ = m.Close()
	listener, err := net.Listen("tcp", address)
	if err != nil {
		m.setError(err.Error())
		return fmt.Errorf("reverse proxy listen %s: %w", address, err)
	}
	server := &http.Server{
		Handler:           m,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      5 * time.Minute,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    32 << 10,
		ErrorLog:          log.New(discardWriter{}, "", 0),
	}
	m.mu.Lock()
	m.listener, m.server, m.address, m.lastError = listener, server, address, ""
	m.mu.Unlock()
	go func() {
		if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			m.mu.Lock()
			if m.server == server {
				m.lastError = err.Error()
				m.listener = nil
			}
			m.mu.Unlock()
		}
	}()
	return nil
}

func (m *Manager) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	request.Body = http.MaxBytesReader(writer, request.Body, 64<<20)
	host := request.Host
	if parsedHost, _, err := net.SplitHostPort(request.Host); err == nil {
		host = parsedHost
	}
	host = validate.CanonicalHostname(host)
	if !validate.LocalHostname(host) {
		http.Error(writer, "Unknown OmaStack route", http.StatusMisdirectedRequest)
		return
	}
	m.mu.RLock()
	target, ok := m.routes[host]
	resolver := m.resolve
	m.mu.RUnlock()
	if !ok {
		http.Error(writer, "OmaStack route not found", http.StatusNotFound)
		return
	}
	port, available := target.port, target.port > 0
	if resolver != nil {
		port, available = resolver(target.serviceID, target.port)
	}
	if !available || !validate.Port(port) {
		http.Error(writer, "OmaStack target is not running", http.StatusBadGateway)
		return
	}
	// Count by stable service ID, not hostname: reconfiguration must not reset
	// in-flight admission or allow aliases to consume every global slot.
	m.mu.Lock()
	if m.active[target.serviceID] >= 16 {
		m.mu.Unlock()
		http.Error(writer, "OmaStack route is busy", http.StatusServiceUnavailable)
		return
	}
	select {
	case m.requests <- struct{}{}:
		m.active[target.serviceID]++
	default:
		m.mu.Unlock()
		http.Error(writer, "OmaStack proxy is busy", http.StatusServiceUnavailable)
		return
	}
	m.mu.Unlock()
	defer func() {
		m.mu.Lock()
		m.active[target.serviceID]--
		if m.active[target.serviceID] == 0 {
			delete(m.active, target.serviceID)
		}
		m.mu.Unlock()
		<-m.requests
	}()
	upstream := &url.URL{Scheme: "http", Host: net.JoinHostPort("127.0.0.1", strconv.Itoa(port))}
	reverse := httputil.NewSingleHostReverseProxy(upstream)
	reverse.Transport = m.transport
	reverse.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		http.Error(w, "OmaStack upstream unavailable", http.StatusBadGateway)
	}
	reverse.Director = func(out *http.Request) {
		out.URL.Scheme = upstream.Scheme
		out.URL.Host = upstream.Host
		out.Host = request.Host
		sanitizeForwardingHeaders(out.Header, request.Host)
	}
	reverse.ServeHTTP(writer, request)
}

func sanitizeForwardingHeaders(headers http.Header, originalHost string) {
	for name := range headers {
		lower := strings.ToLower(name)
		if lower == "forwarded" || lower == "x-real-ip" || strings.HasPrefix(lower, "x-forwarded-") || strings.HasPrefix(lower, "x-original-") {
			headers.Del(name)
		}
	}
	// A nil slice tells ReverseProxy not to append the local client's IP.
	headers["X-Forwarded-For"] = nil
	headers.Set("X-Forwarded-Host", originalHost)
	headers.Set("X-Forwarded-Proto", "http")
}

func (m *Manager) Routes() []model.ActiveRoute {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]model.ActiveRoute, 0, len(m.routes))
	for host, target := range m.routes {
		port, active := target.port, target.port > 0
		if m.resolve != nil {
			port, active = m.resolve(target.serviceID, target.port)
		}
		route := model.ActiveRoute{Hostname: host, URL: "http://" + net.JoinHostPort(host, strconv.Itoa(m.port)), Active: active && validate.Port(port) && m.listener != nil}
		if port > 0 {
			route.Target = net.JoinHostPort("127.0.0.1", strconv.Itoa(port))
		}
		if m.listener == nil {
			route.Error = "proxy is disabled or its listener is unavailable"
		} else if !route.Active {
			route.Error = "target is not running or has no detected port"
		}
		result = append(result, route)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Hostname < result[j].Hostname })
	return result
}

func (m *Manager) Close() error {
	m.transport.CloseIdleConnections()
	m.mu.Lock()
	server := m.server
	m.server, m.listener, m.address = nil, nil, ""
	m.mu.Unlock()
	if server == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	return server.Shutdown(ctx)
}

func (m *Manager) LastError() string     { m.mu.RLock(); defer m.mu.RUnlock(); return m.lastError }
func (m *Manager) setError(value string) { m.mu.Lock(); m.lastError = value; m.mu.Unlock() }

type discardWriter struct{}

func (discardWriter) Write(value []byte) (int, error) { return len(value), nil }
