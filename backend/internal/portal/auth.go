package portal

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"slices"
	"strings"
	"sync"
	"time"
)

// OAuthConfig describes alisa-cloud's opaque-token OAuth protocol, not OIDC.
type OAuthConfig struct {
	Issuer, Origin, ClientID, ClientSecret, ResourceClientID, ResourceClientSecret, TenantID string
	AuthorizationURL, TokenURL, IntrospectionURL, PermissionURL, RevocationURL               string
	AllowLoopback, AllowAPIToken                                                             bool
}
type authSession struct {
	Token, User, CSRF string
	Expires           time.Time
	CanManage         bool
}
type loginAttempt struct {
	State, Verifier string
	Destination     string
	Expires         time.Time
}
type CloudAuth struct {
	config   OAuthConfig
	client   *http.Client
	mu       sync.Mutex
	sessions map[[32]byte]authSession
	attempts map[[32]byte]loginAttempt
}

const consoleScope = "portal:console:manage"

func OAuthFromEnv() (*CloudAuth, error) {
	if os.Getenv("ALISA_OAUTH_ISSUER") == "" {
		for _, key := range []string{"ALISA_OAUTH_CLIENT_ID", "ALISA_OAUTH_CLIENT_SECRET", "ALISA_OAUTH_RESOURCE_CLIENT_ID", "ALISA_OAUTH_RESOURCE_CLIENT_SECRET"} {
			if os.Getenv(key) != "" {
				return nil, errors.New("ALISA_OAUTH_ISSUER is required when OAuth credentials are configured")
			}
		}
		return nil, nil
	}
	return NewCloudAuth(OAuthConfig{
		Issuer: os.Getenv("ALISA_OAUTH_ISSUER"), Origin: os.Getenv("ALISA_PORTAL_ORIGIN"), ClientID: os.Getenv("ALISA_OAUTH_CLIENT_ID"), ClientSecret: os.Getenv("ALISA_OAUTH_CLIENT_SECRET"), ResourceClientID: os.Getenv("ALISA_OAUTH_RESOURCE_CLIENT_ID"), ResourceClientSecret: os.Getenv("ALISA_OAUTH_RESOURCE_CLIENT_SECRET"), TenantID: os.Getenv("ALISA_OAUTH_TENANT_ID"),
		AuthorizationURL: os.Getenv("ALISA_OAUTH_AUTHORIZATION_URL"), TokenURL: os.Getenv("ALISA_OAUTH_TOKEN_URL"), IntrospectionURL: os.Getenv("ALISA_OAUTH_INTROSPECTION_URL"), PermissionURL: os.Getenv("ALISA_OAUTH_PERMISSION_URL"), RevocationURL: os.Getenv("ALISA_OAUTH_REVOCATION_URL"),
		AllowLoopback: os.Getenv("ALISA_OAUTH_ALLOW_LOOPBACK") == "true", AllowAPIToken: os.Getenv("ALISA_OAUTH_ALLOW_API_TOKEN") == "true",
	})
}
func NewCloudAuth(c OAuthConfig) (*CloudAuth, error) {
	c.Issuer = strings.TrimRight(c.Issuer, "/")
	c.Origin = strings.TrimRight(c.Origin, "/")
	for _, raw := range []string{c.Issuer, c.Origin} {
		if !oauthURL(raw, c.AllowLoopback) {
			return nil, errors.New("OAuth issuer and portal origin require HTTPS (explicit loopback HTTP is development only)")
		}
	}
	origin, _ := url.Parse(c.Origin)
	if origin.Path != "" {
		return nil, errors.New("portal origin must not contain a path")
	}
	if c.ClientID == "" || c.ClientSecret == "" || c.ResourceClientID == "" || c.ResourceClientSecret == "" {
		return nil, errors.New("OAuth application and introspection client configuration is incomplete")
	}
	endpoints := []struct {
		value *string
		path  string
	}{{&c.AuthorizationURL, "/oauth2/authorize"}, {&c.TokenURL, "/oauth2/token"}, {&c.IntrospectionURL, "/oauth2/introspect"}, {&c.PermissionURL, "/api/v1/authorization/check"}, {&c.RevocationURL, "/oauth2/revoke"}}
	for _, e := range endpoints {
		if *e.value == "" {
			*e.value = c.Issuer + e.path
		}
		if !oauthURL(*e.value, c.AllowLoopback) {
			return nil, errors.New("OAuth endpoints require HTTPS")
		}
	}
	return &CloudAuth{config: c, client: &http.Client{Timeout: 3 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}, sessions: map[[32]byte]authSession{}, attempts: map[[32]byte]loginAttempt{}}, nil
}
func oauthURL(raw string, allow bool) bool {
	u, e := url.Parse(raw)
	if e != nil || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return false
	}
	if u.Scheme == "https" {
		return true
	}
	ip := net.ParseIP(u.Hostname())
	return allow && u.Scheme == "http" && (u.Hostname() == "localhost" || ip != nil && ip.IsLoopback())
}
func randomSecret() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return base64.RawURLEncoding.EncodeToString(b)
}
func (a *CloudAuth) cookieName(kind string) string {
	if strings.HasPrefix(a.config.Origin, "https://") {
		return "__Host-alisa-portal-" + kind
	}
	return "alisa-portal-" + kind
}
func (a *CloudAuth) cookie(w http.ResponseWriter, kind, value string, maxAge int) {
	http.SetCookie(w, &http.Cookie{Name: a.cookieName(kind), Value: value, Path: "/", MaxAge: maxAge, Secure: strings.HasPrefix(a.config.Origin, "https://"), HttpOnly: true, SameSite: http.SameSiteLaxMode})
}
func (a *CloudAuth) cleanup() {
	now := time.Now()
	for k, v := range a.attempts {
		if !v.Expires.After(now) {
			delete(a.attempts, k)
		}
	}
	for k, v := range a.sessions {
		if !v.Expires.After(now) {
			delete(a.sessions, k)
		}
	}
}
func (a *CloudAuth) exchange(ctx context.Context, endpoint string, form url.Values, id, secret string, out any) error {
	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if secret != "" {
		req.SetBasicAuth(url.QueryEscape(id), url.QueryEscape(secret))
	}
	return a.send(req, out)
}

type upstreamStatus int

func (status upstreamStatus) Error() string {
	return fmt.Sprintf("authorization service returned %d", status)
}

func (a *CloudAuth) send(req *http.Request, out any) error {
	res, err := a.client.Do(req)
	if err != nil {
		return errors.New("authorization service unavailable")
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return upstreamStatus(res.StatusCode)
	}
	if out == nil {
		return nil
	}
	return json.NewDecoder(io.LimitReader(res.Body, 64<<10)).Decode(out)
}

type tokenClaims struct {
	Active   bool            `json:"active"`
	Issuer   string          `json:"iss"`
	App      string          `json:"application_id"`
	Client   string          `json:"client_id"`
	User     string          `json:"userId"`
	Identity string          `json:"identity_type"`
	Tenant   string          `json:"tenant_id"`
	Scope    string          `json:"scope"`
	Audience json.RawMessage `json:"aud"`
	Expires  int64           `json:"exp"`
}

func (a *CloudAuth) introspect(ctx context.Context, token string) (tokenClaims, int) {
	var claims tokenClaims
	if err := a.exchange(ctx, a.config.IntrospectionURL, url.Values{"token": {token}}, a.config.ResourceClientID, a.config.ResourceClientSecret, &claims); err != nil {
		return tokenClaims{}, 503
	}
	if !claims.Active || claims.Expires <= time.Now().Unix() {
		return tokenClaims{}, 401
	}
	var audience []string
	if json.Unmarshal(claims.Audience, &audience) != nil {
		var v string
		_ = json.Unmarshal(claims.Audience, &v)
		audience = []string{v}
	}
	audienceOK := false
	for _, v := range audience {
		if v == "portal" {
			audienceOK = true
		}
	}
	if claims.Issuer != a.config.Issuer || claims.App != "portal" || !audienceOK || claims.Tenant != a.config.TenantID {
		return tokenClaims{}, 403
	}
	return claims, 200
}

func (a *CloudAuth) permission(ctx context.Context, token, resource, id, action string) (bool, int) {
	payload, _ := json.Marshal(map[string]string{"applicationId": "portal", "resourceType": resource, "resourceId": id, "action": action})
	body := strings.NewReader(string(payload))
	req, err := http.NewRequestWithContext(ctx, "POST", a.config.PermissionURL, body)
	if err != nil {
		return false, 503
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	var decision struct {
		Code int `json:"code"`
		Data struct {
			Effect string `json:"effect"`
		} `json:"data"`
	}
	if err = a.send(req, &decision); err != nil {
		if err == upstreamStatus(http.StatusUnauthorized) || err == upstreamStatus(http.StatusForbidden) {
			return false, int(err.(upstreamStatus))
		}
		return false, 503
	}
	if decision.Code != 200 || (decision.Data.Effect != "PERMIT" && decision.Data.Effect != "DENY") {
		return false, 403
	}
	return decision.Data.Effect == "PERMIT", 200
}

func (a *CloudAuth) validate(ctx context.Context, token string) (authSession, int) {
	claims, status := a.introspect(ctx, token)
	if status != 200 {
		return authSession{}, status
	}
	if claims.Identity != "user" || claims.User == "" || !slices.Contains(strings.Fields(claims.Scope), consoleScope) {
		return authSession{}, 403
	}
	canManage, status := a.permission(ctx, token, "console", "*", "manage")
	if status != 200 {
		return authSession{}, status
	}
	if !canManage && a.config.TenantID == "" {
		return authSession{}, 403
	}
	return authSession{Token: token, User: claims.User, Expires: time.Unix(claims.Expires, 0), CanManage: canManage}, 200
}
func (a *CloudAuth) session(r *http.Request) (authSession, string, bool) {
	c, err := r.Cookie(a.cookieName("session"))
	if err != nil {
		return authSession{}, "", false
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.cleanup()
	s, ok := a.sessions[sha256.Sum256([]byte(c.Value))]
	return s, c.Value, ok
}
func (a *CloudAuth) sameOrigin(r *http.Request) bool {
	return r.Header.Get("Origin") == a.config.Origin
}
func (a *CloudAuth) authorized(w http.ResponseWriter, r *http.Request) bool {
	token := ""
	var s authSession
	var sessionID string
	if header := r.Header.Get("Authorization"); header != "" {
		if a.config.AllowAPIToken {
			local := os.Getenv("API_TOKEN")
			if local != "" && subtle.ConstantTimeCompare([]byte(header), []byte("Bearer "+local)) == 1 {
				return true
			}
		}
		if !strings.HasPrefix(header, "Bearer ") {
			fail(w, 401, "请先使用统一账号登录")
			return false
		}
		token = strings.TrimPrefix(header, "Bearer ")
	} else {
		var ok bool
		s, sessionID, ok = a.session(r)
		if !ok {
			fail(w, 401, "请先使用统一账号登录")
			return false
		}
		token = s.Token
		if r.Method != "GET" && r.Method != "HEAD" && (!a.sameOrigin(r) || subtle.ConstantTimeCompare([]byte(r.Header.Get("X-CSRF-Token")), []byte(s.CSRF)) != 1) {
			fail(w, 403, "请求校验失败，请刷新页面")
			return false
		}
	}
	if token == "" || len(token) > 8192 {
		fail(w, 401, "无效的访问令牌")
		return false
	}
	validated, status := a.validate(r.Context(), token)
	if status != 200 {
		if sessionID != "" && status != 503 {
			a.mu.Lock()
			delete(a.sessions, sha256.Sum256([]byte(sessionID)))
			a.mu.Unlock()
			a.cookie(w, "session", "", -1)
		}
		messages := map[int]string{401: "登录已失效，请重新登录", 403: "当前账号没有门户管理权限", 503: "授权服务暂不可用，请稍后重试"}
		fail(w, status, messages[status])
		return false
	}
	if !validated.CanManage {
		fail(w, 403, "当前账号没有门户管理权限")
		return false
	}
	return true
}
func (a *CloudAuth) register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/auth/config", func(w http.ResponseWriter, r *http.Request) {
		respond(w, 200, map[string]any{"enabled": true, "allowApiToken": a.config.AllowAPIToken})
	})
	mux.HandleFunc("GET /api/auth/login", a.login)
	mux.HandleFunc("GET /api/auth/callback", a.callback)
	mux.HandleFunc("GET /api/auth/session", func(w http.ResponseWriter, r *http.Request) {
		s, id, ok := a.session(r)
		if !ok {
			respond(w, 200, map[string]any{"authenticated": false})
			return
		}
		validated, status := a.validate(r.Context(), s.Token)
		if status != 200 {
			if status == 503 {
				fail(w, 503, "授权服务暂不可用")
				return
			}
			a.mu.Lock()
			delete(a.sessions, sha256.Sum256([]byte(id)))
			a.mu.Unlock()
			a.cookie(w, "session", "", -1)
			respond(w, 200, map[string]any{"authenticated": false, "error": "登录失效或缺少门户管理权限"})
			return
		}
		respond(w, 200, map[string]any{"authenticated": true, "user": s.User, "csrfToken": s.CSRF, "canManage": validated.CanManage})
	})
	mux.HandleFunc("POST /api/auth/logout", func(w http.ResponseWriter, r *http.Request) {
		s, id, ok := a.session(r)
		if !a.sameOrigin(r) || ok && subtle.ConstantTimeCompare([]byte(r.Header.Get("X-CSRF-Token")), []byte(s.CSRF)) != 1 {
			fail(w, 403, "请求校验失败")
			return
		}
		a.mu.Lock()
		delete(a.sessions, sha256.Sum256([]byte(id)))
		a.mu.Unlock()
		a.cookie(w, "session", "", -1)
		if ok {
			_ = a.exchange(r.Context(), a.config.RevocationURL, url.Values{"token": {s.Token}, "client_id": {a.config.ClientID}}, a.config.ClientID, a.config.ClientSecret, nil)
		}
		respond(w, 200, map[string]bool{"ok": true})
	})
}
func (a *CloudAuth) login(w http.ResponseWriter, r *http.Request) {
	destination := "/"
	if r.URL.Query().Get("next") == "admin-services" {
		destination = "/admin/portal-services"
	}
	attempt := loginAttempt{State: randomSecret(), Verifier: randomSecret(), Destination: destination, Expires: time.Now().Add(10 * time.Minute)}
	id := randomSecret()
	a.mu.Lock()
	a.cleanup()
	if len(a.attempts) >= 1024 {
		a.mu.Unlock()
		fail(w, 503, "登录请求过多，请稍后重试")
		return
	}
	a.attempts[sha256.Sum256([]byte(id))] = attempt
	a.mu.Unlock()
	a.cookie(w, "login", id, 600)
	challenge := sha256.Sum256([]byte(attempt.Verifier))
	u, _ := url.Parse(a.config.AuthorizationURL)
	q := u.Query()
	q.Set("response_type", "code")
	q.Set("client_id", a.config.ClientID)
	q.Set("redirect_uri", a.config.Origin+"/api/auth/callback")
	q.Set("scope", consoleScope)
	q.Set("state", attempt.State)
	q.Set("code_challenge", base64.RawURLEncoding.EncodeToString(challenge[:]))
	q.Set("code_challenge_method", "S256")
	u.RawQuery = q.Encode()
	http.Redirect(w, r, u.String(), http.StatusFound)
}
func (a *CloudAuth) callback(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Referrer-Policy", "no-referrer")
	c, err := r.Cookie(a.cookieName("login"))
	if err != nil {
		fail(w, 400, "登录校验失败，请重新发起登录")
		return
	}
	a.mu.Lock()
	attempt, ok := a.attempts[sha256.Sum256([]byte(c.Value))]
	delete(a.attempts, sha256.Sum256([]byte(c.Value)))
	a.mu.Unlock()
	a.cookie(w, "login", "", -1)
	if !ok || !attempt.Expires.After(time.Now()) || subtle.ConstantTimeCompare([]byte(attempt.State), []byte(r.URL.Query().Get("state"))) != 1 {
		fail(w, 400, "登录校验失败，请重新发起登录")
		return
	}
	deny := func(reason string) { http.Redirect(w, r, a.config.Origin+"/?auth_error="+reason, http.StatusSeeOther) }
	code := r.URL.Query().Get("code")
	if code == "" || len(code) > 4096 || r.URL.Query().Get("error") != "" {
		deny("cancelled")
		return
	}
	form := url.Values{"grant_type": {"authorization_code"}, "code": {code}, "client_id": {a.config.ClientID}, "redirect_uri": {a.config.Origin + "/api/auth/callback"}, "code_verifier": {attempt.Verifier}}
	var response struct {
		AccessToken string `json:"access_token"`
		Type        string `json:"token_type"`
		Expires     int64  `json:"expires_in"`
	}
	if err = a.exchange(r.Context(), a.config.TokenURL, form, a.config.ClientID, a.config.ClientSecret, &response); err != nil || response.AccessToken == "" || !strings.EqualFold(response.Type, "Bearer") {
		deny("unavailable")
		return
	}
	s, status := a.validate(r.Context(), response.AccessToken)
	if status != 200 {
		if status == 503 {
			deny("unavailable")
		} else {
			deny("forbidden")
		}
		return
	}
	s.CSRF = randomSecret()
	if max := time.Now().Add(8 * time.Hour); s.Expires.After(max) {
		s.Expires = max
	}
	id := randomSecret()
	a.mu.Lock()
	a.cleanup()
	if len(a.sessions) >= 2048 {
		a.mu.Unlock()
		deny("unavailable")
		return
	}
	if old, e := r.Cookie(a.cookieName("session")); e == nil {
		delete(a.sessions, sha256.Sum256([]byte(old.Value)))
	}
	a.sessions[sha256.Sum256([]byte(id))] = s
	a.mu.Unlock()
	a.cookie(w, "session", id, int(time.Until(s.Expires).Seconds()))
	http.Redirect(w, r, a.config.Origin+attempt.Destination, http.StatusSeeOther)
}
func publicAuthPath(r *http.Request) bool {
	switch r.URL.Path {
	case "/api/auth/config", "/api/auth/session", "/api/auth/login", "/api/auth/callback":
		return r.Method == "GET"
	case "/api/auth/logout":
		return r.Method == "POST"
	}
	return false
}
