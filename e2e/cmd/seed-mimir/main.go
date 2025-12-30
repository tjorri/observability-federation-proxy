// seed-mimir pushes sample metrics to Mimir for e2e testing.
package main

import (
	"bytes"
	"flag"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gogo/protobuf/proto"
	"github.com/golang/snappy"
	"github.com/prometheus/prometheus/prompb"
)

func main() {
	var (
		url     string
		tenants string
	)

	flag.StringVar(&url, "url", "http://localhost:8080", "Mimir URL")
	flag.StringVar(&tenants, "tenants", "tenant-alpha,tenant-beta,tenant-gamma", "Comma-separated list of tenants")
	flag.Parse()

	tenantList := strings.Split(tenants, ",")
	now := time.Now().UnixMilli()

	for _, tenant := range tenantList {
		tenant = strings.TrimSpace(tenant)
		fmt.Printf("==> Pushing metrics for tenant: %s\n", tenant)

		// Create sample metrics
		writeReq := &prompb.WriteRequest{
			Timeseries: []prompb.TimeSeries{
				{
					Labels: []prompb.Label{
						{Name: "__name__", Value: "e2e_test_counter_total"},
						{Name: "job", Value: "e2e-test"},
						{Name: "namespace", Value: tenant},
						{Name: "instance", Value: "test-instance:9090"},
					},
					Samples: []prompb.Sample{
						{Value: 100, Timestamp: now - 60000},
						{Value: 150, Timestamp: now - 30000},
						{Value: 200, Timestamp: now},
					},
				},
				{
					Labels: []prompb.Label{
						{Name: "__name__", Value: "e2e_test_gauge"},
						{Name: "job", Value: "e2e-test"},
						{Name: "namespace", Value: tenant},
						{Name: "instance", Value: "test-instance:9090"},
					},
					Samples: []prompb.Sample{
						{Value: 42.5, Timestamp: now - 60000},
						{Value: 43.2, Timestamp: now - 30000},
						{Value: 41.8, Timestamp: now},
					},
				},
				{
					Labels: []prompb.Label{
						{Name: "__name__", Value: "up"},
						{Name: "job", Value: "e2e-test"},
						{Name: "namespace", Value: tenant},
						{Name: "instance", Value: "test-instance:9090"},
					},
					Samples: []prompb.Sample{
						{Value: 1, Timestamp: now},
					},
				},
			},
		}

		if err := pushMetrics(url, tenant, writeReq); err != nil {
			fmt.Printf("    Error pushing metrics for %s: %v\n", tenant, err)
		} else {
			fmt.Printf("    Successfully pushed metrics for %s\n", tenant)
		}
	}
}

func pushMetrics(baseURL, tenant string, req *prompb.WriteRequest) error {
	data, err := proto.Marshal(req)
	if err != nil {
		return fmt.Errorf("failed to marshal write request: %w", err)
	}

	compressed := snappy.Encode(nil, data)

	httpReq, err := http.NewRequest("POST", baseURL+"/api/v1/push", bytes.NewReader(compressed))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/x-protobuf")
	httpReq.Header.Set("Content-Encoding", "snappy")
	httpReq.Header.Set("X-Prometheus-Remote-Write-Version", "0.1.0")
	httpReq.Header.Set("X-Scope-OrgID", tenant)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	return nil
}
