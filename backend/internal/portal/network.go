package portal

import (
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
)

type NetworkZone struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Scope       string `json:"scope"`
	CIDR        string `json:"cidr"`
	Gateway     string `json:"gateway"`
	DNS         string `json:"dns"`
	Description string `json:"description"`
}
type NetworkSettings struct {
	Revision      int           `json:"revision"`
	InternalHosts []string      `json:"internalHosts"`
	ExternalHosts []string      `json:"externalHosts"`
	Networks      []NetworkZone `json:"networks"`
}

func (s *Store) normalize() {
	for i := range s.state.Services {
		v := &s.state.Services[i]
		if v.ExternalURL == "" {
			v.ExternalURL = v.URL
		}
	}
	n := &s.state.NetworkSettings
	if n.InternalHosts == nil {
		n.InternalHosts = []string{}
	}
	if n.ExternalHosts == nil {
		n.ExternalHosts = []string{}
	}
	if n.Networks == nil {
		n.Networks = []NetworkZone{}
	}
}
func validWebURL(raw string) bool {
	if raw == "" {
		return true
	}
	if len(raw) > 2048 {
		return false
	}
	u, e := url.Parse(raw)
	return e == nil && u.Hostname() != "" && (u.Scheme == "http" || u.Scheme == "https") && u.User == nil
}
func validateService(v *Service) error {
	v.Name = strings.TrimSpace(v.Name)
	if v.Name == "" || len(v.Name) > 80 || len(v.Description) > 500 || len(v.Category) > 40 {
		return fmt.Errorf("请填写有效服务名称、分类和说明")
	}
	for _, raw := range []string{v.URL, v.InternalURL, v.ExternalURL} {
		if !validWebURL(raw) {
			return fmt.Errorf("内外网地址必须是 HTTP(S) 地址且不能包含凭据")
		}
	}
	if v.ExternalURL == "" && v.URL != "" {
		v.ExternalURL = v.URL
	}
	v.URL = v.ExternalURL // Compatibility for older clients; internal endpoints never masquerade as public URLs.
	return nil
}
func (s *Store) validNetwork(id string) bool {
	if id == "" {
		return true
	}
	for _, n := range s.state.NetworkSettings.Networks {
		if n.ID == id {
			return true
		}
	}
	return false
}
func validateHosts(hosts []string) error {
	if len(hosts) > 100 {
		return fmt.Errorf("入口主机规则最多 100 条")
	}
	seen := map[string]bool{}
	for i, h := range hosts {
		h = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(h), "."))
		hosts[i] = h
		if h == "" || len(h) > 253 || strings.ContainsAny(h, "/:@* ?#\\") || seen[h] {
			return fmt.Errorf("请填写不含协议、端口或路径的唯一入口主机名；IPv6 地址由系统自动识别")
		}
		if net.ParseIP(h) == nil {
			for _, label := range strings.Split(h, ".") {
				if len(label) == 0 || len(label) > 63 || strings.HasPrefix(label, "-") || strings.HasSuffix(label, "-") {
					return fmt.Errorf("入口主机名格式无效")
				}
				for _, c := range label {
					if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '-') {
						return fmt.Errorf("入口主机名格式无效")
					}
				}
			}
		}
		seen[h] = true
	}
	return nil
}
func registerNetworkRoutes(mux *http.ServeMux, s *Store) {
	mux.HandleFunc("PUT /api/services/{id}", func(w http.ResponseWriter, r *http.Request) {
		var v Service
		if !decode(w, r, &v) {
			return
		}
		if err := validateService(&v); err != nil {
			fail(w, 400, err.Error())
			return
		}
		s.mu.Lock()
		defer s.mu.Unlock()
		if !s.tenant(v.Tenant) || !s.validNetwork(v.NetworkID) {
			fail(w, 400, "租户或网络区域不存在")
			return
		}
		next := s.copy()
		for i, old := range next.Services {
			if old.ID == r.PathValue("id") {
				v.ID = old.ID
				v.Status = old.Status
				next.Services[i] = v
				s.commit(w, next, "更新服务入口 · "+v.Name)
				return
			}
		}
		fail(w, 404, "服务不存在")
	})
	mux.HandleFunc("PUT /api/network-settings", func(w http.ResponseWriter, r *http.Request) {
		var v NetworkSettings
		if !decode(w, r, &v) {
			return
		}
		if err := validateHosts(v.InternalHosts); err != nil {
			fail(w, 400, err.Error())
			return
		}
		if err := validateHosts(v.ExternalHosts); err != nil {
			fail(w, 400, err.Error())
			return
		}
		seenHosts := map[string]bool{}
		for _, h := range v.InternalHosts {
			seenHosts[h] = true
		}
		for _, h := range v.ExternalHosts {
			if seenHosts[h] {
				fail(w, 400, "同一入口不能同时属于内网和外网")
				return
			}
		}
		if len(v.Networks) > 100 {
			fail(w, 400, "网络区域最多 100 个")
			return
		}
		seen := map[string]bool{}
		for i := range v.Networks {
			n := &v.Networks[i]
			if n.ID == "" {
				n.ID = id()
			}
			if seen[n.ID] || len(n.ID) > 80 || strings.TrimSpace(n.Name) == "" || len(n.Name) > 80 || len(n.Description) > 500 || len(n.Gateway) > 256 || len(n.DNS) > 256 || (n.Scope != "internal" && n.Scope != "external" && n.Scope != "tunnel") {
				fail(w, 400, "请检查网络区域名称、类型和说明")
				return
			}
			seen[n.ID] = true
			if n.CIDR != "" {
				if _, _, err := net.ParseCIDR(n.CIDR); err != nil {
					fail(w, 400, "网段需使用 CIDR 格式，例如 192.168.1.0/24")
					return
				}
			}
		}
		s.mu.Lock()
		defer s.mu.Unlock()
		if v.Revision != s.state.NetworkSettings.Revision {
			fail(w, 409, "网络配置已更新，请刷新后重试")
			return
		}
		for _, service := range s.state.Services {
			if service.NetworkID != "" && !seen[service.NetworkID] {
				fail(w, 409, "该网络区域仍有关联服务，请先调整服务归属")
				return
			}
		}
		v.Revision++
		if v.InternalHosts == nil {
			v.InternalHosts = []string{}
		}
		if v.ExternalHosts == nil {
			v.ExternalHosts = []string{}
		}
		if v.Networks == nil {
			v.Networks = []NetworkZone{}
		}
		next := s.copy()
		next.NetworkSettings = v
		s.commit(w, next, "更新网络区域与内外网选址规则")
	})
}
