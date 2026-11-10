package portal

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

type Service struct {
	Visibility        string `json:"visibility"`
	AccessRequirement string `json:"accessRequirement"`
	PublicDescription string `json:"publicDescription"`
	ID                string `json:"id"`
	Name              string `json:"name"`
	Category          string `json:"category"`
	Tenant            string `json:"tenant"`
	URL               string `json:"url"`
	InternalURL       string `json:"internalUrl"`
	ExternalURL       string `json:"externalUrl"`
	NetworkID         string `json:"networkId"`
	Status            string `json:"status"`
	Description       string `json:"description"`
}
type Tenant struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	CPU       int    `json:"cpu"`
	Memory    int    `json:"memory"`
}
type Deployment struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Tenant   string `json:"tenant"`
	Image    string `json:"image"`
	Replicas int    `json:"replicas"`
	Port     int    `json:"port"`
	Created  string `json:"created"`
	Status   string `json:"status"`
	Manifest string `json:"manifest"`
}
type Event struct {
	Message string `json:"message"`
	Time    string `json:"time"`
}
type State struct {
	CatalogVersion  int             `json:"catalogVersion"`
	NetworkSettings NetworkSettings `json:"networkSettings"`
	Services        []Service       `json:"services"`
	Tenants         []Tenant        `json:"tenants"`
	Deployments     []Deployment    `json:"deployments"`
	Events          []Event         `json:"events"`
}
type Store struct {
	mu    sync.Mutex
	path  string
	state State
}

func Open(path string) (*Store, error) {
	if path == "" {
		path = "data/portal.json"
	}
	s := &Store{path: path}
	data, err := os.ReadFile(path)
	if err == nil {
		err = json.Unmarshal(data, &s.state)
		if err == nil {
			s.normalize()
		}
		return s, err
	}
	if !os.IsNotExist(err) {
		return nil, err
	}
	s.state = State{Services: []Service{}, Tenants: []Tenant{{"platform", "平台团队", "platform", 8, 16}}, Deployments: []Deployment{}, Events: []Event{}}
	for _, v := range []struct{ name, category, desc string }{{"Traefik", "网关", "集群入口与 HTTP 路由"}, {"SeaweedFS", "存储", "对象存储与文件服务"}, {"PostgreSQL", "数据库", "关系型数据存储"}, {"MySQL", "数据库", "业务数据库"}, {"Registry", "镜像仓库", "容器镜像分发"}, {"Argo CD", "部署", "GitOps 持续交付"}, {"Nacos", "配置中心", "服务发现与配置管理"}, {"Gitea", "开发工具", "代码仓库与协作"}} {
		s.state.Services = append(s.state.Services, Service{ID: id(), Name: v.name, Category: v.category, Tenant: "platform", Status: "unverified", Description: v.desc})
	}
	s.normalize()
	return s, nil
}
func id() string {
	b := make([]byte, 12)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}
func (s *Store) save(next State) error {
	data, err := json.MarshalIndent(next, "", "  ")
	if err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Dir(s.path), 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(s.path), ".portal-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	if err = os.Rename(f.Name(), s.path); err != nil {
		return err
	}
	s.state = next
	return nil
}
func respond(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func fail(w http.ResponseWriter, status int, msg string) {
	respond(w, status, map[string]string{"error": msg})
}
func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if err := d.Decode(v); err != nil {
		fail(w, 400, "请求格式不正确")
		return false
	}
	if err := d.Decode(&struct{}{}); err != io.EOF {
		fail(w, 400, "仅允许一个 JSON 对象")
		return false
	}
	return true
}

var dns = regexp.MustCompile(`^[a-z][a-z0-9-]{0,61}[a-z0-9]$`)
var imagePattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9./_:@-]{0,254}$`)

func Handler(s *Store) http.Handler { return HandlerWithAuth(s, nil) }
func HandlerWithAuth(s *Store, auth *CloudAuth) http.Handler {
	mux := http.NewServeMux()
	registerPublicRoutes(mux, s)
	if auth != nil {
		auth.register(mux)
	} else {
		mux.HandleFunc("GET /api/auth/config", func(w http.ResponseWriter, r *http.Request) {
			respond(w, 200, map[string]any{"enabled": false, "allowApiToken": true})
		})
		mux.HandleFunc("GET /api/auth/session", func(w http.ResponseWriter, r *http.Request) { respond(w, 200, map[string]bool{"authenticated": false}) })
	}
	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, r *http.Request) { respond(w, 200, map[string]string{"status": "ok"}) })
	mux.HandleFunc("GET /api/state", func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		defer s.mu.Unlock()
		respond(w, 200, s.state)
	})
	mux.HandleFunc("GET /api/metrics", metrics)
	registerResourceWrite(mux, auth, "POST /api/services", "service", "create", func(w http.ResponseWriter, r *http.Request) {
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
		if !s.validNetwork(v.NetworkID) {
			fail(w, 400, "网络区域不存在")
			return
		}
		if v.Tenant == "" {
			v.Tenant = "platform"
		}
		if !s.tenant(v.Tenant) {
			fail(w, 400, "命名空间未登记")
			return
		}
		v.ID = id()
		v.Status = "unverified"
		next := s.copy()
		next.Services = append(next.Services, v)
		s.commit(w, r, next, v, "登记服务 · "+v.Name)
	})
	registerResourceWrite(mux, auth, "DELETE /api/services/{id}", "service", "delete", func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		defer s.mu.Unlock()
		next := s.copy()
		for i, v := range next.Services {
			if v.ID == r.PathValue("id") {
				next.Services = append(next.Services[:i], next.Services[i+1:]...)
				s.commit(w, r, next, nil, "移除服务 · "+v.Name)
				return
			}
		}
		fail(w, 404, "服务不存在")
	})
	registerResourceWrite(mux, auth, "POST /api/tenants", "tenant", "create", func(w http.ResponseWriter, r *http.Request) {
		var v Tenant
		if !decode(w, r, &v) {
			return
		}
		if strings.TrimSpace(v.Name) == "" || len(v.Name) > 80 || !dns.MatchString(v.Namespace) {
			fail(w, 400, "名称不能为空，命名空间需为 2–63 位小写 DNS 名称")
			return
		}
		s.mu.Lock()
		defer s.mu.Unlock()
		for _, t := range s.state.Tenants {
			if t.Namespace == v.Namespace {
				fail(w, 409, "命名空间已存在")
				return
			}
		}
		v.ID = id()
		next := s.copy()
		next.Tenants = append(next.Tenants, v)
		s.commit(w, r, next, v, "登记命名空间 · "+v.Name)
	})
	registerResourceWrite(mux, auth, "POST /api/deployments", "deployment", "create", func(w http.ResponseWriter, r *http.Request) {
		var v Deployment
		if !decode(w, r, &v) {
			return
		}
		if !dns.MatchString(v.Name) || !imagePattern.MatchString(v.Image) || v.Replicas < 1 || v.Replicas > 20 || v.Port < 1 || v.Port > 65535 {
			fail(w, 400, "请检查应用名、镜像、端口和副本数（1–20）")
			return
		}
		s.mu.Lock()
		defer s.mu.Unlock()
		var tenant Tenant
		for _, t := range s.state.Tenants {
			if t.ID == v.Tenant {
				tenant = t
			}
		}
		if tenant.ID == "" {
			fail(w, 400, "命名空间未登记")
			return
		}
		for _, d := range s.state.Deployments {
			if d.Name == v.Name && d.Tenant == v.Tenant {
				fail(w, 409, "该命名空间已存在同名部署申请")
				return
			}
		}
		v.ID = id()
		v.Created = time.Now().UTC().Format(time.RFC3339)
		v.Status = "draft"
		v.Manifest = manifest(v, tenant)
		next := s.copy()
		next.Deployments = append(next.Deployments, v)
		s.commit(w, r, next, v, "生成部署清单 · "+v.Name)
	})
	registerNetworkRoutes(mux, s, auth)
	registerResourceReads(mux, s, auth)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Cache-Control", "no-store")
		if !strings.HasPrefix(r.URL.Path, "/api/oauth/") && r.URL.Path != "/api/health" && !(r.Method == http.MethodGet && r.URL.Path == "/api/public/services") && !publicAuthPath(r) {
			if auth != nil {
				if !auth.authorized(w, r) {
					return
				}
				mux.ServeHTTP(w, r)
				return
			}
			token := os.Getenv("API_TOKEN")
			if token != "" && subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), []byte("Bearer "+token)) != 1 {
				fail(w, 401, "需要管理员访问令牌")
				return
			}
		}
		mux.ServeHTTP(w, r)
	})
}
func (s *Store) tenant(id string) bool {
	for _, t := range s.state.Tenants {
		if t.ID == id {
			return true
		}
	}
	return false
}
func (s *Store) copy() State {
	data, _ := json.Marshal(s.state)
	var next State
	_ = json.Unmarshal(data, &next)
	return next
}
func (s *Store) commit(w http.ResponseWriter, r *http.Request, next State, resource any, msg string) {
	next.Events = append([]Event{{msg, time.Now().UTC().Format(time.RFC3339)}}, next.Events...)
	if len(next.Events) > 100 {
		next.Events = next.Events[:100]
	}
	if err := s.save(next); err != nil {
		fail(w, 500, "保存失败，请检查数据目录权限")
		return
	}
	if strings.HasPrefix(r.URL.Path, "/api/oauth/") {
		if resource == nil {
			w.WriteHeader(http.StatusNoContent)
		} else if r.Method == http.MethodPost {
			respond(w, http.StatusCreated, resource)
		} else {
			respond(w, http.StatusOK, resource)
		}
		return
	}
	respond(w, 201, next)
}
func manifest(d Deployment, t Tenant) string {
	return fmt.Sprintf(`apiVersion: apps/v1
kind: Deployment
metadata:
  name: %s
  namespace: %s
spec:
  replicas: %d
  selector:
    matchLabels: {app: %s}
  template:
    metadata:
      labels: {app: %s}
    spec:
      containers:
        - name: app
          image: %q
          ports:
            - containerPort: %d
          resources:
            requests: {cpu: 100m, memory: 128Mi}
            limits: {cpu: 250m, memory: 256Mi}
---
apiVersion: v1
kind: Service
metadata:
  name: %s
  namespace: %s
spec:
  selector: {app: %s}
  ports:
    - port: 80
      targetPort: %d
`, d.Name, t.Namespace, d.Replicas, d.Name, d.Name, d.Image, d.Port, d.Name, t.Namespace, d.Name, d.Port)
}
