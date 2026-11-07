package portal

import (
	"encoding/json"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestWorkflowPersistenceAndValidation(t *testing.T) {
	t.Setenv("API_TOKEN", "")
	path := filepath.Join(t.TempDir(), "state.json")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	h := Handler(s)
	call := func(method, path, body string, want int) *httptest.ResponseRecorder {
		t.Helper()
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(method, path, strings.NewReader(body)))
		if w.Code != want {
			t.Fatalf("%s %s: %d %s", method, path, w.Code, w.Body.String())
		}
		return w
	}
	call("POST", "/api/tenants", `{"name":"研发","namespace":"team-dev","cpu":2,"memory":4}`, 201)
	call("POST", "/api/tenants", `{"name":"重复","namespace":"team-dev","cpu":2,"memory":4}`, 409)
	call("POST", "/api/deployments", `{"name":"test-app","tenant":"missing","image":"nginx:alpine","replicas":1,"port":80}`, 400)
	w := call("POST", "/api/deployments", `{"name":"test-app","tenant":"platform","image":"nginx:alpine","replicas":2,"port":80}`, 201)
	var state State
	if err := json.Unmarshal(w.Body.Bytes(), &state); err != nil {
		t.Fatal(err)
	}
	if len(state.Deployments) != 1 || !strings.Contains(state.Deployments[0].Manifest, "namespace: platform") {
		t.Fatal("manifest missing tenant namespace")
	}
	call("POST", "/api/deployments", `{"name":"test-app","tenant":"platform","image":"nginx:alpine","replicas":1,"port":80}`, 409)
	call("POST", "/api/services", `{"name":"x","tenant":"platform","url":"javascript:alert(1)"}`, 400)
	call("POST", "/api/services", `{"name":"x","tenant":"platform","url":"https://user:pass@example.com"}`, 400)
	call("POST", "/api/services", `{"name":"Redis","tenant":"platform","category":"数据库"}`, 201)
	reloaded, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(reloaded.state.Deployments) != 1 || len(reloaded.state.Tenants) != 2 {
		t.Fatal("state not persisted")
	}
	call("DELETE", "/api/services/unknown", "", 404)
}
func TestAuthAndQuota(t *testing.T) {
	t.Setenv("API_TOKEN", "test-secret")
	s, _ := Open(filepath.Join(t.TempDir(), "state.json"))
	h := Handler(s)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/api/state", nil))
	if w.Code != 401 {
		t.Fatal("unauthorized request allowed")
	}
	req := httptest.NewRequest("GET", "/api/state", nil)
	req.Header.Set("Authorization", "Bearer test-secret")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatal("valid token rejected")
	}
	t.Setenv("API_TOKEN", "")
	s.state.Tenants[0].CPU = 1
	req = httptest.NewRequest("POST", "/api/deployments", strings.NewReader(`{"name":"large-app","tenant":"platform","image":"nginx:alpine","replicas":5,"port":80}`))
	w = httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != 409 {
		t.Fatal("quota exceeded without rejection")
	}
}
