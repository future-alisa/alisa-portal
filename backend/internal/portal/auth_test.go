package portal

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

type authFixture struct {
	mu          sync.Mutex
	claims      map[string]any
	effect      string
	unavailable bool
	challenge   string
	revoked     bool
	a           *CloudAuth
	h           http.Handler
	server      *httptest.Server
	t           *testing.T
}

func newAuthFixture(t *testing.T) *authFixture {
	t.Helper()
	f := &authFixture{t: t, effect: "PERMIT"}
	f.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		if f.unavailable {
			w.WriteHeader(503)
			return
		}
		_ = r.ParseForm()
		switch r.URL.Path {
		case "/oauth2/token":
			id, secret, ok := r.BasicAuth()
			if !ok || id != "portal-client" || secret != "client-secret" {
				t.Error("missing confidential client authentication")
				w.WriteHeader(401)
				return
			}
			hash := sha256.Sum256([]byte(r.Form.Get("code_verifier")))
			if base64.RawURLEncoding.EncodeToString(hash[:]) != f.challenge || r.Form.Get("code") != "one-time-code" || r.Form.Get("redirect_uri") != "https://portal.example/api/auth/callback" {
				t.Error("invalid PKCE exchange")
				w.WriteHeader(400)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "opaque-secret-token", "token_type": "Bearer", "expires_in": 300})
		case "/oauth2/introspect":
			id, secret, ok := r.BasicAuth()
			if !ok || id != "portal-resource" || secret != "resource-secret" {
				t.Error("missing resource credentials")
				w.WriteHeader(401)
				return
			}
			if r.Form.Get("token") != "opaque-secret-token" {
				_ = json.NewEncoder(w).Encode(map[string]any{"active": false})
				return
			}
			_ = json.NewEncoder(w).Encode(f.claims)
		case "/api/v1/authorization/check":
			var body map[string]string
			_ = json.NewDecoder(r.Body).Decode(&body)
			if r.Header.Get("Authorization") != "Bearer opaque-secret-token" || body["applicationId"] != "portal" || body["resourceType"] != "console" || body["resourceId"] != "*" || body["action"] != "manage" {
				t.Error("incorrect permission boundary")
				w.WriteHeader(403)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 200, "data": map[string]string{"effect": f.effect}})
		case "/oauth2/revoke":
			f.revoked = true
		default:
			w.WriteHeader(404)
		}
	}))
	t.Cleanup(f.server.Close)
	f.claims = map[string]any{"active": true, "exp": time.Now().Add(time.Hour).Unix(), "iss": f.server.URL, "application_id": "portal", "aud": []string{"portal"}, "identity_type": "user", "userId": "user-1", "scope": consoleScope, "tenant_id": ""}
	a, err := NewCloudAuth(OAuthConfig{Issuer: f.server.URL, Origin: "https://portal.example", ClientID: "portal-client", ClientSecret: "client-secret", ResourceClientID: "portal-resource", ResourceClientSecret: "resource-secret", AllowLoopback: true})
	if err != nil {
		t.Fatal(err)
	}
	f.a = a
	store, err := Open(filepath.Join(t.TempDir(), "portal.json"))
	if err != nil {
		t.Fatal(err)
	}
	f.h = HandlerWithAuth(store, a)
	return f
}
func (f *authFixture) call(method, path string, cookie *http.Cookie, headers map[string]string) *httptest.ResponseRecorder {
	f.t.Helper()
	r := httptest.NewRequest(method, path, nil)
	if cookie != nil {
		r.AddCookie(cookie)
	}
	for k, v := range headers {
		r.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	f.h.ServeHTTP(w, r)
	return w
}
func cookieNamed(t *testing.T, w *httptest.ResponseRecorder, name string) *http.Cookie {
	t.Helper()
	for _, c := range w.Result().Cookies() {
		if c.Name == name {
			return c
		}
	}
	t.Fatalf("cookie %s missing", name)
	return nil
}
func (f *authFixture) start() (*http.Cookie, string) {
	f.t.Helper()
	w := f.call("GET", "/api/auth/login", nil, nil)
	if w.Code != 302 {
		f.t.Fatal(w.Code)
	}
	u, err := url.Parse(w.Header().Get("Location"))
	if err != nil {
		f.t.Fatal(err)
	}
	if u.Query().Get("code_challenge_method") != "S256" || u.Query().Get("scope") != consoleScope || u.Query().Get("redirect_uri") != "https://portal.example/api/auth/callback" {
		f.t.Fatal(u)
	}
	f.mu.Lock()
	f.challenge = u.Query().Get("code_challenge")
	f.mu.Unlock()
	return cookieNamed(f.t, w, f.a.cookieName("login")), u.Query().Get("state")
}
func (f *authFixture) login() *http.Cookie {
	f.t.Helper()
	cookie, state := f.start()
	w := f.call("GET", "/api/auth/callback?code=one-time-code&state="+url.QueryEscape(state), cookie, nil)
	if w.Code != 303 || w.Header().Get("Location") != "https://portal.example/" {
		f.t.Fatal(w.Code, w.Header(), w.Body.String())
	}
	c := cookieNamed(f.t, w, f.a.cookieName("session"))
	if !c.Secure || !c.HttpOnly || c.SameSite != http.SameSiteLaxMode || c.Path != "/" {
		f.t.Fatal("unsafe session cookie")
	}
	return c
}
func TestCloudLoginSessionCSRFAndLogout(t *testing.T) {
	t.Setenv("API_TOKEN", "legacy-token")
	f := newAuthFixture(t)
	for _, path := range []string{"/api/auth/config", "/api/auth/session", "/api/public/services"} {
		if w := f.call("GET", path, nil, nil); w.Code != 200 {
			t.Fatal(path, w.Code)
		}
	}
	if w := f.call("GET", "/api/state", nil, map[string]string{"Authorization": "Bearer legacy-token", "X-User-Header": "forged"}); w.Code != 401 {
		t.Fatal(w.Code)
	}
	cookie := f.login()
	w := f.call("GET", "/api/auth/session", cookie, nil)
	var data struct {
		Authenticated bool   `json:"authenticated"`
		User          string `json:"user"`
		CSRF          string `json:"csrfToken"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &data)
	if !data.Authenticated || data.User != "user-1" || data.CSRF == "" || strings.Contains(w.Body.String(), "opaque-secret-token") {
		t.Fatal(w.Body.String())
	}
	if w = f.call("GET", "/api/state", cookie, nil); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	for _, headers := range []map[string]string{nil, {"Origin": "https://evil.example", "X-CSRF-Token": data.CSRF}, {"Origin": "https://portal.example"}} {
		if w = f.call("POST", "/api/services", cookie, headers); w.Code != 403 {
			t.Fatal("missing CSRF protection", w.Code)
		}
	}
	headers := map[string]string{"Origin": "https://portal.example", "X-CSRF-Token": data.CSRF}
	if w = f.call("POST", "/api/services", cookie, headers); w.Code != 400 {
		t.Fatal("valid CSRF should reach body validation", w.Code)
	}
	if w = f.call("POST", "/api/auth/logout", cookie, headers); w.Code != 200 {
		t.Fatal(w.Code)
	}
	if w = f.call("GET", "/api/state", cookie, nil); w.Code != 401 {
		t.Fatal("logout left local session active", w.Code)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.revoked {
		t.Fatal("upstream revocation not called")
	}
}
func TestStateIsBoundToLoginBrowserAndOneUse(t *testing.T) {
	f := newAuthFixture(t)
	cookie, state := f.start()
	if w := f.call("GET", "/api/auth/callback?code=one-time-code&state="+state, nil, nil); w.Code != 400 {
		t.Fatal(w.Code)
	}
	if w := f.call("GET", "/api/auth/callback?code=one-time-code&state=wrong", cookie, nil); w.Code != 400 {
		t.Fatal(w.Code)
	}
	if w := f.call("GET", "/api/auth/callback?code=one-time-code&state="+state, cookie, nil); w.Code != 400 {
		t.Fatal("callback replay succeeded")
	}
}
func TestOpaqueTokenPermissionBoundaries(t *testing.T) {
	cases := []struct {
		name, key string
		value     any
		want      int
	}{
		{"revoked", "active", false, 401}, {"expired", "exp", int64(1), 401}, {"wrong issuer", "iss", "https://attacker.example", 403}, {"wrong app", "application_id", "smallokr", 403}, {"wrong audience", "aud", []string{"smallokr"}, 403}, {"machine identity", "identity_type", "application", 403}, {"missing user", "userId", "", 403}, {"wrong tenant", "tenant_id", "other-tenant", 403}, {"scope missing", "scope", "portal:service:read", 403},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newAuthFixture(t)
			f.mu.Lock()
			f.claims[tc.key] = tc.value
			f.mu.Unlock()
			w := f.call("GET", "/api/state", nil, map[string]string{"Authorization": "Bearer opaque-secret-token"})
			if w.Code != tc.want {
				t.Fatal(w.Code, w.Body.String())
			}
		})
	}
	for _, effect := range []string{"DENY", "INDETERMINATE", ""} {
		t.Run(effect, func(t *testing.T) {
			f := newAuthFixture(t)
			f.mu.Lock()
			f.effect = effect
			f.mu.Unlock()
			if w := f.call("GET", "/api/state", nil, map[string]string{"Authorization": "Bearer opaque-secret-token"}); w.Code != 403 {
				t.Fatal(w.Code)
			}
		})
	}
}
func TestRevocationIsCheckedEveryRequestAndOutageFailsClosed(t *testing.T) {
	f := newAuthFixture(t)
	cookie := f.login()
	f.mu.Lock()
	f.unavailable = true
	f.mu.Unlock()
	if w := f.call("GET", "/api/state", cookie, nil); w.Code != 503 {
		t.Fatal(w.Code)
	}
	f.mu.Lock()
	f.unavailable = false
	f.claims["active"] = false
	f.mu.Unlock()
	if w := f.call("GET", "/api/state", cookie, nil); w.Code != 401 {
		t.Fatal(w.Code)
	}
	if w := f.call("GET", "/api/auth/session", cookie, nil); strings.Contains(w.Body.String(), `"authenticated":true`) {
		t.Fatal("revoked session still active")
	}
}
func TestOAuthConfigurationRejectsInsecureOrIncompleteSettings(t *testing.T) {
	base := OAuthConfig{Issuer: "https://auth.example", Origin: "https://portal.example", ClientID: "client", ClientSecret: "secret", ResourceClientID: "resource", ResourceClientSecret: "secret"}
	for _, change := range []func(*OAuthConfig){func(c *OAuthConfig) { c.Issuer = "http://192.168.1.1"; c.AllowLoopback = true }, func(c *OAuthConfig) { c.Origin = "https://portal.example/path" }, func(c *OAuthConfig) { c.ResourceClientSecret = "" }, func(c *OAuthConfig) { c.TokenURL = "https://user:pass@auth.example/token" }, func(c *OAuthConfig) { c.PermissionURL = "https://auth.example/check?token=bad" }} {
		c := base
		change(&c)
		if _, err := NewCloudAuth(c); err == nil {
			t.Fatal("unsafe configuration accepted")
		}
	}
}

func TestDeniedConsentCallbackCreatesNoSession(t *testing.T) {
	f := newAuthFixture(t)
	cookie, state := f.start()
	f.mu.Lock()
	f.effect = "DENY"
	f.mu.Unlock()
	w := f.call("GET", "/api/auth/callback?code=one-time-code&state="+state, cookie, nil)
	if w.Code != 303 || w.Header().Get("Location") != "https://portal.example/?auth_error=forbidden" {
		t.Fatal(w.Code, w.Header())
	}
	for _, c := range w.Result().Cookies() {
		if c.Name == f.a.cookieName("session") && c.MaxAge > 0 {
			t.Fatal("denied user received session")
		}
	}
	if strings.Contains(w.Body.String(), "opaque-secret-token") {
		t.Fatal("credential leaked")
	}
}
func TestExplicitEmergencyTokenAndOwningTenant(t *testing.T) {
	t.Setenv("API_TOKEN", "emergency-only")
	f := newAuthFixture(t)
	f.a.config.AllowAPIToken = true
	if w := f.call("GET", "/api/state", nil, map[string]string{"Authorization": "Bearer emergency-only"}); w.Code != 200 {
		t.Fatal(w.Code)
	}
	f.a.config.AllowAPIToken = false
	f.a.config.TenantID = "portal-owner"
	f.mu.Lock()
	f.claims["tenant_id"] = "portal-owner"
	f.mu.Unlock()
	if w := f.call("GET", "/api/state", nil, map[string]string{"Authorization": "Bearer opaque-secret-token"}); w.Code != 200 {
		t.Fatal(w.Code)
	}
	f.mu.Lock()
	f.effect = "DENY"
	f.mu.Unlock()
	if w := f.call("GET", "/api/state", nil, map[string]string{"Authorization": "Bearer opaque-secret-token"}); w.Code != 403 {
		t.Fatal("scope bypassed PDP", w.Code)
	}
}
func TestPartialOAuthEnvironmentFailsStartup(t *testing.T) {
	t.Setenv("ALISA_OAUTH_ISSUER", "")
	t.Setenv("ALISA_OAUTH_CLIENT_ID", "configured-client")
	if _, err := OAuthFromEnv(); err == nil {
		t.Fatal("partial configuration silently downgraded to legacy auth")
	}
}

func TestTenantMemberCanSignInWithoutManagementAccess(t *testing.T) {
	f := newAuthFixture(t)
	f.a.config.TenantID = "portal-tenant"
	f.mu.Lock()
	f.claims["tenant_id"] = "portal-tenant"
	f.effect = "DENY"
	f.mu.Unlock()

	cookie := f.login()
	w := f.call("GET", "/api/auth/session", cookie, nil)
	var session struct {
		Authenticated bool `json:"authenticated"`
		CanManage     bool `json:"canManage"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &session); err != nil || w.Code != 200 || !session.Authenticated || session.CanManage {
		t.Fatal(w.Code, w.Body.String())
	}
	if w := f.call("GET", "/api/state", cookie, nil); w.Code != 403 {
		t.Fatal("member accessed management state", w.Code)
	}

	f.mu.Lock()
	f.effect = "PERMIT"
	f.mu.Unlock()
	if w := f.call("GET", "/api/state", cookie, nil); w.Code != 200 {
		t.Fatal("newly authorized member could not access management state", w.Code)
	}
}
