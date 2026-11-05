package cmd

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeMeter serves a P1 reading with the given active power.
func fakeMeter(t *testing.T, activePowerW int) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"smr_version":50,"meter_model":"test","wifi_ssid":"x","wifi_strength":40,
			"total_power_import_t1_kwh":1,"total_power_import_t2_kwh":2,
			"total_power_export_t1_kwh":3,"total_power_export_t2_kwh":4,
			"active_power_w":%d,"active_power_l1_w":0,"active_power_l2_w":0,"active_power_l3_w":0,
			"total_gas_m3":5}`, activePowerW)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

// startExporter runs Start() with the given env and returns its base URL.
func startExporter(t *testing.T, env map[string]string) string {
	t.Helper()
	port := freePort(t)
	env["PORT"] = fmt.Sprint(port)
	env["TICK"] = "1"
	for k, v := range env {
		t.Setenv(k, v)
	}
	go Start()
	return fmt.Sprintf("http://127.0.0.1:%d", port)
}

// waitFor polls url until its body contains want, or fails after 10s.
func waitFor(t *testing.T, url, want string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	var last string
	for time.Now().Before(deadline) {
		if resp, err := http.Get(url); err == nil {
			b, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			last = string(b)
			if strings.Contains(last, want) {
				return
			}
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatalf("%s never contained %q; last body: %.300s", url, want, last)
}

func TestServesMetricsWithoutInfluxDB(t *testing.T) {
	meter := fakeMeter(t, 4321)
	base := startExporter(t, map[string]string{
		"HOST":          strings.TrimPrefix(meter.URL, "http://"),
		"INFLUXDB_HOST": "",
	})

	waitFor(t, base+"/metrics", "4321")
	waitFor(t, base+"/readyz", "ok: last scrape")
}

func TestPushesToInfluxDBWhenConfigured(t *testing.T) {
	meter := fakeMeter(t, 8765)
	var mu sync.Mutex
	var writes []string
	influx := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		writes = append(writes, r.URL.Path+" "+string(b))
		mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(influx.Close)

	base := startExporter(t, map[string]string{
		"HOST":            strings.TrimPrefix(meter.URL, "http://"),
		"INFLUXDB_HOST":   influx.URL,
		"INFLUXDB_TOKEN":  "t",
		"INFLUXDB_ORG":    "o",
		"INFLUXDB_BUCKET": "b",
	})
	waitFor(t, base+"/metrics", "8765")

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		for _, w := range writes {
			if strings.HasPrefix(w, "/api/v2/write ") && strings.Contains(w, "ActivePowerW=8765") {
				mu.Unlock()
				return
			}
		}
		mu.Unlock()
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatalf("no InfluxDB write with ActivePowerW=8765; got %q", writes)
}
