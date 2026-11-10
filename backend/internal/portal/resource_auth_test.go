package portal

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestResourceOAuth(t *testing.T) {
	f := newAuthFixture(t)
	f.claims["scope"] = "portal:service:create"
	f.permission = [3]string{"service", "*", "create"}
	headers := map[string]string{"Authorization": "Bearer opaque-secret-token"}

	r := httptest.NewRequest("POST", "/api/oauth/services", strings.NewReader(`{"name":"Private service","category":"platform","externalUrl":"https://service.example"}`))
	r.Header.Set("Authorization", headers["Authorization"])
	w := httptest.NewRecorder()
	f.h.ServeHTTP(w, r)
	var service Service
	if w.Code != http.StatusCreated || json.Unmarshal(w.Body.Bytes(), &service) != nil || service.ID == "" || strings.Contains(w.Body.String(), "networkSettings") {
		t.Fatalf("expected only the created service, got %d %s", w.Code, w.Body.String())
	}

	check := func(method, path string, headers map[string]string, status int) {
		t.Helper()
		if w := f.call(method, path, nil, headers); w.Code != status {
			t.Fatalf("%s %s got %d, want %d: %s", method, path, w.Code, status, w.Body.String())
		}
	}
	path := "/api/oauth/services/" + service.ID
	check("GET", path, nil, 401)
	check("GET", path, headers, 403)
	check("GET", "/api/state", headers, 403)
	if f.checks != 1 {
		t.Fatalf("requests lacking credentials or scope reached the PDP: %d checks", f.checks)
	}

	f.claims["scope"] = "portal:service:read"
	f.permission = [3]string{"service", service.ID, "read"}
	check("GET", path, headers, 200)
	f.effect = "DENY"
	check("GET", path, headers, 403)

	f.effect = "PERMIT"
	f.claims["identity_type"] = "application"
	f.claims["client_id"] = "automation"
	delete(f.claims, "userId")
	check("GET", path, headers, 403)
	f.a.config.TenantID = "tenant-a"
	f.claims["tenant_id"] = "tenant-b"
	check("GET", path, headers, 403)
	f.claims["tenant_id"] = "tenant-a"
	check("GET", path, headers, 200)
	f.claims["scope"] = consoleScope
	check("GET", "/api/state", headers, 403)
	check("GET", path, headers, 403)
}
