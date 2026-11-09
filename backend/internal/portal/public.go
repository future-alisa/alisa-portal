package portal

import (
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
)

// PublicService is deliberately independent of Service: private fields never cross this boundary.
type PublicService struct {
	ID                string `json:"id"`
	Name              string `json:"name"`
	Category          string `json:"category"`
	URL               string `json:"url"`
	Description       string `json:"description"`
	AccessRequirement string `json:"accessRequirement"`
}

func publicURL(raw string) bool {
	if raw == "" || !validWebURL(raw) {
		return false
	}
	u, _ := url.Parse(raw)
	h := strings.ToLower(strings.TrimSuffix(u.Hostname(), "."))
	if ip := net.ParseIP(h); ip != nil {
		return ip.IsGlobalUnicast() && !ip.IsPrivate()
	}
	return strings.Contains(h, ".") && h != "localhost" && !strings.HasSuffix(h, ".localhost") && !strings.HasSuffix(h, ".local") && !strings.HasSuffix(h, ".internal")
}
func validatePublication(v *Service) error {
	if v.Visibility == "" {
		v.Visibility = "private"
	}
	if v.AccessRequirement == "" {
		v.AccessRequirement = "account"
	}
	if (v.Visibility != "public" && v.Visibility != "private") || (v.AccessRequirement != "none" && v.AccessRequirement != "account") || len(v.PublicDescription) > 500 {
		return fmt.Errorf("请检查公开范围、访问要求和公开说明")
	}
	if v.Visibility == "public" && !publicURL(v.ExternalURL) {
		return fmt.Errorf("公开资源需要有效外网地址，不能使用本地或私有地址")
	}
	return nil
}
func registerPublicRoutes(mux *http.ServeMux, s *Store) {
	mux.HandleFunc("GET /api/public/services", func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		defer s.mu.Unlock()
		result := []PublicService{}
		for _, v := range s.state.Services {
			if v.Visibility != "public" || !publicURL(v.ExternalURL) {
				continue
			}
			requirement := "account"
			if v.AccessRequirement == "none" {
				requirement = "none"
			}
			result = append(result, PublicService{v.ID, v.Name, v.Category, v.ExternalURL, v.PublicDescription, requirement})
		}
		respond(w, 200, map[string]any{"services": result})
	})
}

// Apply the explicitly approved public entries once. Persisting the version with
// state ensures later administrator edits and withdrawals survive restarts.
func (s *Store) initializeVisitorResources() {
	if s.state.CatalogVersion >= 1 {
		return
	}
	for _, entry := range []struct{ name, category, url, description string }{
		{"Alisa Docs", "文档", "https://alisa.vip.cpolar.cn/docs/", "Alisa 的项目文档与使用指南。"},
		{"Alisa Blog", "博客", "https://blog.alisacloud.com", "Alisa 的博客与技术文章。"},
	} {
		found := false
		for i := range s.state.Services {
			v := &s.state.Services[i]
			raw := v.ExternalURL
			if raw == "" {
				raw = v.URL
			}
			if strings.TrimRight(raw, "/") != strings.TrimRight(entry.url, "/") {
				continue
			}
			found = true
			// Respect any previously explicit administrator visibility choice.
			if v.Visibility == "" {
				v.Visibility = "public"
				v.AccessRequirement = "none"
				v.PublicDescription = entry.description
			}
			break
		}
		if !found {
			tenant := ""
			if len(s.state.Tenants) > 0 {
				tenant = s.state.Tenants[0].ID
			}
			s.state.Services = append(s.state.Services, Service{ID: id(), Name: entry.name, Category: entry.category, Tenant: tenant, URL: entry.url, ExternalURL: entry.url, Visibility: "public", AccessRequirement: "none", PublicDescription: entry.description, Status: "unverified"})
		}
	}
	s.state.CatalogVersion = 1
}
