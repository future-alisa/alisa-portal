package portal

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

type Metric struct {
	Key   string   `json:"key"`
	Label string   `json:"label"`
	Value *float64 `json:"value"`
	Unit  string   `json:"unit"`
}

func metrics(w http.ResponseWriter, r *http.Request) {
	base := os.Getenv("PROMETHEUS_URL")
	if base == "" {
		respond(w, 200, map[string]any{"mode": "unconfigured", "message": "尚未连接 Prometheus", "series": []Metric{}})
		return
	}
	u, err := url.Parse(base)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		fail(w, 503, "Prometheus 服务端地址配置无效")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
	defer cancel()
	client := &http.Client{Timeout: 8 * time.Second, CheckRedirect: func(req *http.Request, via []*http.Request) error { return http.ErrUseLastResponse }}
	defs := []struct{ key, label, unit, query string }{{"cpu", "CPU 使用率", "%", `100 * (1 - avg(rate(node_cpu_seconds_total{mode="idle"}[5m])))`}, {"memory", "内存使用率", "%", `100 * (1 - sum(node_memory_MemAvailable_bytes) / sum(node_memory_MemTotal_bytes))`}, {"storage", "存储使用率", "%", `100 * (1 - sum(node_filesystem_avail_bytes{fstype!~"tmpfs|overlay"}) / sum(node_filesystem_size_bytes{fstype!~"tmpfs|overlay"}))`}, {"pods", "运行中 Pod", "个", `sum(kube_pod_status_phase{phase="Running"})`}}
	series := make([]Metric, 0, len(defs))
	for _, d := range defs {
		endpoint := *u
		endpoint.Path = strings.TrimRight(endpoint.Path, "/") + "/api/v1/query"
		q := endpoint.Query()
		q.Set("query", d.query)
		endpoint.RawQuery = q.Encode()
		req, err := http.NewRequestWithContext(ctx, "GET", endpoint.String(), nil)
		if err != nil {
			fail(w, 502, "无法创建监控请求")
			return
		}
		if token := os.Getenv("PROMETHEUS_TOKEN"); token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		res, err := client.Do(req)
		if err != nil {
			fail(w, 502, "Prometheus 暂时不可用")
			return
		}
		var body struct {
			Status string `json:"status"`
			Data   struct {
				Result []struct {
					Value []json.RawMessage `json:"value"`
				} `json:"result"`
			} `json:"data"`
		}
		err = json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(&body)
		res.Body.Close()
		if err != nil || res.StatusCode != 200 || body.Status != "success" {
			fail(w, 502, "Prometheus 查询失败")
			return
		}
		m := Metric{Key: d.key, Label: d.label, Unit: d.unit}
		if len(body.Data.Result) > 0 && len(body.Data.Result[0].Value) == 2 {
			var raw string
			if json.Unmarshal(body.Data.Result[0].Value[1], &raw) == nil {
				value, e := strconv.ParseFloat(raw, 64)
				if e == nil && !math.IsNaN(value) && !math.IsInf(value, 0) {
					m.Value = &value
				}
			}
		}
		series = append(series, m)
	}
	respond(w, 200, map[string]any{"mode": "connected", "message": fmt.Sprintf("Prometheus · %s", time.Now().UTC().Format(time.RFC3339)), "series": series})
}
