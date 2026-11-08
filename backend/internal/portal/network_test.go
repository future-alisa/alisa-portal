package portal

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLegacyMigrationAndNetworkUpdates(t *testing.T) {
	t.Setenv("API_TOKEN", "")
	path := filepath.Join(t.TempDir(), "portal.json")
	legacy := `{"services":[{"id":"stable","name":"Legacy","tenant":"platform","url":"https://public.example.com","status":"unverified"}],"tenants":[{"id":"platform","name":"Platform","namespace":"platform","cpu":8,"memory":16}],"deployments":[],"events":[]}`
	if err := os.WriteFile(path, []byte(legacy), 0600); err != nil {
		t.Fatal(err)
	}
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if s.state.Services[0].ExternalURL != "https://public.example.com" || s.state.NetworkSettings.Networks == nil {
		t.Fatal("legacy data migration failed")
	}
	h := Handler(s)
	call := func(method, path, body string, want int) {
		t.Helper()
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(method, path, strings.NewReader(body)))
		if w.Code != want {
			t.Fatalf("%s %s: %d %s", method, path, w.Code, w.Body.String())
		}
	}
	call("PUT", "/api/network-settings", `{"revision":0,"internalHosts":["portal.lan"],"externalHosts":["portal.example.com"],"networks":[{"id":"lan","name":"LAN","scope":"internal","cidr":"192.168.1.0/24"}]}`, 201)
	call("PUT", "/api/services/stable", `{"name":"Legacy","tenant":"platform","internalUrl":"https://service.local","externalUrl":"https://public.example.com","networkId":"lan"}`, 201)
	if s.state.Services[0].ID != "stable" || s.state.Services[0].InternalURL != "https://service.local" {
		t.Fatal("service update replaced identity or lost internal URL")
	}
	call("PUT", "/api/network-settings", `{"revision":0}`, 409)
	call("PUT", "/api/network-settings", `{"revision":1,"networks":[]}`, 409)
	call("PUT", "/api/network-settings", `{"revision":1,"internalHosts":["same.example.com"],"externalHosts":["same.example.com"]}`, 400)
	call("PUT", "/api/network-settings", `{"revision":1,"internalHosts":["https://portal.local/path"]}`, 400)
	call("PUT", "/api/network-settings", `{"revision":1,"networks":[{"name":"bad","scope":"internal","cidr":"999.1.1.0/24"}]}`, 400)
	call("PUT", "/api/services/stable", `{"name":"Legacy","tenant":"platform","internalUrl":"javascript:alert(1)"}`, 400)
	call("PUT", "/api/services/stable", `{"name":"Legacy","tenant":"platform","internalUrl":"https://user:pass@service.local"}`, 400)
	call("PUT", "/api/services/stable", `{"name":"Legacy","tenant":"platform","externalUrl":"https://public.example.com","networkId":"missing"}`, 400)
	call("PUT", "/api/services/stable", `{"name":"Legacy","tenant":"platform","internalUrl":"http://192.168.1.10:8080"}`, 201)
	if s.state.Services[0].URL != "" || s.state.Services[0].ExternalURL != "" {
		t.Fatal("clearing external URL restored a stale public URL")
	}
	call("PUT", "/api/network-settings", `{"revision":1,"internalHosts":["PORTAL.LAN."],"externalHosts":[],"networks":[]}`, 201)
	loaded, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.state.Services[0].InternalURL != "http://192.168.1.10:8080" || loaded.state.NetworkSettings.Revision != 2 {
		t.Fatal("network data not persisted")
	}
	var body map[string]any
	data, _ := json.Marshal(loaded.state)
	_ = json.Unmarshal(data, &body)
	if body["networkSettings"] == nil {
		t.Fatal("missing network settings in state")
	}
}
