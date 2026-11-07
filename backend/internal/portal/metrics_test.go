package portal

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestPrometheusMetrics(t *testing.T) {
	t.Setenv("PROMETHEUS_URL", "https://metrics.example.com/prometheus")
	t.Setenv("PROMETHEUS_TOKEN", "metrics-secret")
	previous := http.DefaultTransport
	defer func() { http.DefaultTransport = previous }()
	calls := 0
	http.DefaultTransport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.URL.Path != "/prometheus/api/v1/query" || r.URL.Query().Get("query") == "" {
			t.Fatal("invalid Prometheus query")
		}
		if r.Header.Get("Authorization") != "Bearer metrics-secret" {
			t.Fatal("missing upstream token")
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"status":"success","data":{"result":[{"value":[123,"42.5"]}]}}`)), Header: make(http.Header)}, nil
	})
	w := httptest.NewRecorder()
	metrics(w, httptest.NewRequest("GET", "/api/metrics", nil))
	if w.Code != 200 || calls != 4 {
		t.Fatalf("unexpected response: %d %s", w.Code, w.Body.String())
	}
	var result struct {
		Mode   string   `json:"mode"`
		Series []Metric `json:"series"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Mode != "connected" || len(result.Series) != 4 || result.Series[0].Value == nil || *result.Series[0].Value != 42.5 {
		t.Fatal("invalid metrics result")
	}
	http.DefaultTransport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 500, Body: io.NopCloser(strings.NewReader(`{}`)), Header: make(http.Header)}, nil
	})
	w = httptest.NewRecorder()
	metrics(w, httptest.NewRequest("GET", "/api/metrics", nil))
	if w.Code != 502 {
		t.Fatal("upstream failure not propagated")
	}
	t.Setenv("PROMETHEUS_URL", "")
	w = httptest.NewRecorder()
	metrics(w, httptest.NewRequest("GET", "/api/metrics", nil))
	if !strings.Contains(w.Body.String(), "unconfigured") {
		t.Fatal("unconfigured state missing")
	}
}
