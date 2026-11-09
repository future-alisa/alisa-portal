package portal

import (
	"encoding/json"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestPublicCatalogBoundary(t *testing.T) {
	t.Setenv("API_TOKEN", "admin-secret")
	s, err := Open(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	s.state.Services = []Service{
		{ID: "public", Name: "App", Visibility: "public", ExternalURL: "https://apps.example.com", AccessRequirement: "none", PublicDescription: "Public summary", InternalURL: "http://10.1.1.1", Description: "private-note", Tenant: "private-tenant", NetworkID: "private-network"},
		{ID: "account", Name: "Members", Visibility: "public", ExternalURL: "https://members.example.com", AccessRequirement: "account"},
		{ID: "private", Name: "Secret", ExternalURL: "https://secret.example.com"},
		{ID: "unsafe", Name: "Internal", Visibility: "public", ExternalURL: "http://192.168.1.1"},
	}
	h := Handler(s)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/api/public/services", nil))
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	var result struct {
		Services []PublicService `json:"services"`
	}
	if err = json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Services) != 2 || result.Services[0].AccessRequirement != "none" || result.Services[1].AccessRequirement != "account" {
		t.Fatal(result)
	}
	for _, secret := range []string{"private-note", "private-tenant", "private-network", "10.1.1.1", "Secret", "192.168", "internalUrl", "networkSettings"} {
		if strings.Contains(w.Body.String(), secret) {
			t.Fatalf("leak: %s", secret)
		}
	}
	for _, path := range []string{"/api/state", "/api/metrics", "/api/public/services/extra"} {
		w = httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if w.Code != 401 {
			t.Fatalf("%s: %d", path, w.Code)
		}
	}
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("POST", "/api/public/services", nil))
	if w.Code != 401 {
		t.Fatal(w.Code)
	}
}
func TestPublicationValidation(t *testing.T) {
	v := Service{Name: "Legacy"}
	if err := validateService(&v); err != nil {
		t.Fatal(err)
	}
	if v.Visibility != "private" || v.AccessRequirement != "account" {
		t.Fatal(v)
	}
	for _, raw := range []string{"", "http://localhost", "http://10.1.2.3", "http://[::1]", "http://[fc00::1]", "http://service.local", "http://registry.kind.internal", "https://user:pass@example.com", "javascript:alert(1)"} {
		v = Service{Name: "Test", Visibility: "public", ExternalURL: raw}
		if validateService(&v) == nil {
			t.Fatalf("allowed %s", raw)
		}
	}
	v = Service{Name: "App", Visibility: "public", AccessRequirement: "none", ExternalURL: "https://app.example.com"}
	if err := validateService(&v); err != nil {
		t.Fatal(err)
	}
}

func TestPublishAndWithdrawPersists(t *testing.T) {
	t.Setenv("API_TOKEN", "admin-secret")
	path := filepath.Join(t.TempDir(), "portal.json")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	h := Handler(s)
	call := func(method, route, body string, authenticated bool) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequest(method, route, strings.NewReader(body))
		if authenticated {
			r.Header.Set("Authorization", "Bearer admin-secret")
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	body := `{"name":"App","tenant":"platform","visibility":"public","accessRequirement":"none","externalUrl":"https://app.example.com","publicDescription":"Welcome"}`
	if w := call("POST", "/api/services", body, false); w.Code != 401 {
		t.Fatal(w.Code)
	}
	if w := call("POST", "/api/services", body, true); w.Code != 201 {
		t.Fatal(w.Code, w.Body.String())
	}
	service := s.state.Services[len(s.state.Services)-1]
	loaded, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if v := loaded.state.Services[len(loaded.state.Services)-1]; v.Visibility != "public" || v.PublicDescription != "Welcome" || v.AccessRequirement != "none" {
		t.Fatal(v)
	}
	if w := call("GET", "/api/public/services", "", false); !strings.Contains(w.Body.String(), "Welcome") {
		t.Fatal(w.Body.String())
	}
	body = strings.Replace(body, `"visibility":"public"`, `"visibility":"private"`, 1)
	if w := call("PUT", "/api/services/"+service.ID, body, true); w.Code != 201 {
		t.Fatal(w.Code, w.Body.String())
	}
	if w := call("GET", "/api/public/services", "", false); strings.Contains(w.Body.String(), "Welcome") {
		t.Fatal("withdrawn resource remains public")
	}
}
