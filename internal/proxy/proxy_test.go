package proxy

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"omastack/internal/model"
)

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
