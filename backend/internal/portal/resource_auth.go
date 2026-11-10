package portal

import (
	"net/http"
	"slices"
	"strings"
)

func registerResourceWrite(mux *http.ServeMux, auth *CloudAuth, pattern, resource, action string, handler http.HandlerFunc) {
	mux.HandleFunc(pattern, handler)
	registerResourceAPI(mux, auth, pattern, resource, action, handler)
}

func registerResourceAPI(mux *http.ServeMux, auth *CloudAuth, pattern, resource, action string, handler http.HandlerFunc) {
	if auth == nil {
		return
	}
	pattern = strings.Replace(pattern, " /api/", " /api/oauth/", 1)
	mux.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
		id := "*"
		if strings.HasSuffix(pattern, "/{id}") {
			id = r.PathValue("id")
			if id == "*" {
				fail(w, http.StatusNotFound, "资源不存在")
				return
			}
		}
		status := auth.authorizeResource(r, resource, id, action)
		if status != http.StatusOK {
			messages := map[int]string{401: "需要有效的 OAuth 访问令牌", 403: "当前身份没有该资源权限", 503: "授权服务暂不可用"}
			fail(w, status, messages[status])
			return
		}
		handler(w, r)
	})
}

func (a *CloudAuth) authorizeResource(r *http.Request, resource, id, action string) int {
	header := r.Header.Get("Authorization")
	token, ok := strings.CutPrefix(header, "Bearer ")
	if !ok || token == "" || len(token) > 8192 {
		return http.StatusUnauthorized
	}
	claims, status := a.introspect(r.Context(), token)
	if status != http.StatusOK {
		return status
	}
	switch claims.Identity {
	case "user":
		if strings.TrimSpace(claims.User) == "" {
			return http.StatusForbidden
		}
	case "application":
		if a.config.TenantID == "" || strings.TrimSpace(claims.Client) == "" || claims.User != "" {
			return http.StatusForbidden
		}
	default:
		return http.StatusForbidden
	}
	if !slices.Contains(strings.Fields(claims.Scope), "portal:"+resource+":"+action) {
		return http.StatusForbidden
	}
	allowed, status := a.permission(r.Context(), token, resource, id, action)
	if status == http.StatusOK && !allowed {
		return http.StatusForbidden
	}
	return status
}

func registerResourceReads(mux *http.ServeMux, s *Store, auth *CloudAuth) {
	for _, route := range []struct {
		path, resource string
		selectData     func(State) any
	}{
		{"services", "service", func(state State) any { return state.Services }},
		{"tenants", "tenant", func(state State) any { return state.Tenants }},
		{"deployments", "deployment", func(state State) any { return state.Deployments }},
		{"network-settings", "network", func(state State) any { return state.NetworkSettings }},
		{"events", "event", func(state State) any { return state.Events }},
	} {
		registerResourceAPI(mux, auth, "GET /api/"+route.path, route.resource, "read", func(w http.ResponseWriter, _ *http.Request) {
			s.mu.Lock()
			defer s.mu.Unlock()
			respond(w, http.StatusOK, route.selectData(s.state))
		})
	}
	registerResourceAPI(mux, auth, "GET /api/services/{id}", "service", "read", func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		defer s.mu.Unlock()
		for _, service := range s.state.Services {
			if service.ID == r.PathValue("id") {
				respond(w, http.StatusOK, service)
				return
			}
		}
		fail(w, http.StatusNotFound, "服务不存在")
	})
	registerResourceAPI(mux, auth, "GET /api/metrics", "metrics", "read", metrics)
}
