package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// HTTPRequestsTotal counts total HTTP requests by method, path, and status code.
	HTTPRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "http_requests_total",
			Help: "Total number of HTTP requests",
		},
		[]string{"method", "path", "status"},
	)

	// HTTPRequestDuration measures HTTP request duration in seconds.
	HTTPRequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "http_request_duration_seconds",
			Help:    "HTTP request duration in seconds",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"method", "path"},
	)

	// HTTPRequestsInFlight tracks the number of in-flight HTTP requests.
	HTTPRequestsInFlight = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "http_requests_in_flight",
			Help: "Number of HTTP requests currently being processed",
		},
	)

	// ProxyRequestsTotal counts total proxy requests by cluster and backend type.
	ProxyRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "proxy_requests_total",
			Help: "Total number of proxy requests to backend clusters",
		},
		[]string{"cluster", "backend", "status"},
	)

	// ProxyRequestDuration measures proxy request duration in seconds.
	ProxyRequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "proxy_request_duration_seconds",
			Help:    "Proxy request duration to backend clusters in seconds",
			Buckets: []float64{0.01, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30},
		},
		[]string{"cluster", "backend"},
	)

	// ClusterHealthStatus tracks cluster health status (1 = healthy, 0 = unhealthy).
	ClusterHealthStatus = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "cluster_health_status",
			Help: "Cluster health status (1 = healthy, 0 = unhealthy)",
		},
		[]string{"cluster"},
	)

	// TenantCount tracks the number of discovered tenants per cluster.
	TenantCount = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "tenant_count",
			Help: "Number of discovered tenants per cluster",
		},
		[]string{"cluster"},
	)

	// ClusterInfo provides static cluster configuration info.
	ClusterInfo = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "cluster_info",
			Help: "Cluster configuration information",
		},
		[]string{"cluster", "type", "has_loki", "has_mimir"},
	)
)

// RecordClusterInfo records static cluster configuration.
func RecordClusterInfo(cluster, clusterType string, hasLoki, hasMimir bool) {
	lokiStr := "false"
	if hasLoki {
		lokiStr = "true"
	}
	mimirStr := "false"
	if hasMimir {
		mimirStr = "true"
	}
	ClusterInfo.WithLabelValues(cluster, clusterType, lokiStr, mimirStr).Set(1)
}

// RecordClusterHealth records cluster health status.
func RecordClusterHealth(cluster string, healthy bool) {
	value := 0.0
	if healthy {
		value = 1.0
	}
	ClusterHealthStatus.WithLabelValues(cluster).Set(value)
}

// RecordTenantCount records the number of tenants for a cluster.
func RecordTenantCount(cluster string, count int) {
	TenantCount.WithLabelValues(cluster).Set(float64(count))
}
