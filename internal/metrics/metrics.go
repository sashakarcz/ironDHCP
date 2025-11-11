package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Metrics holds all Prometheus metrics
type Metrics struct {
	// DHCP request metrics
	DHCPRequests *prometheus.CounterVec
	DHCPResponses *prometheus.CounterVec
	DHCPErrors *prometheus.CounterVec

	// Lease metrics
	ActiveLeases prometheus.Gauge
	ExpiredLeases prometheus.Gauge
	TotalLeases prometheus.Counter
	LeaseRenewals prometheus.Counter
	LeaseReleases prometheus.Counter
	LeaseDeclines prometheus.Counter

	// IP allocation metrics
	IPAllocations prometheus.Counter
	IPAllocationErrors prometheus.Counter
	IPAllocationDuration prometheus.Histogram

	// Cluster/HA metrics
	IPAllocationsPerServer *prometheus.CounterVec
	AllocationRetries *prometheus.HistogramVec
	DatabaseLatency *prometheus.HistogramVec

	// Reservation metrics
	StaticReservations prometheus.Gauge

	// Git sync metrics
	GitSyncs *prometheus.CounterVec
	GitSyncDuration prometheus.Histogram
	GitSyncLastTimestamp prometheus.Gauge

	// Database metrics
	DatabaseQueries *prometheus.CounterVec
	DatabaseErrors prometheus.Counter
	DatabaseConnections prometheus.Gauge
}

// New creates and registers all metrics
func New() *Metrics {
	m := &Metrics{
		// DHCP request metrics
		DHCPRequests: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "irondhcp_requests_total",
				Help: "Total number of DHCP requests by message type",
			},
			[]string{"type"}, // DISCOVER, REQUEST, RELEASE, DECLINE, INFORM
		),

		DHCPResponses: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "irondhcp_responses_total",
				Help: "Total number of DHCP responses by message type",
			},
			[]string{"type"}, // OFFER, ACK, NAK
		),

		DHCPErrors: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "irondhcp_errors_total",
				Help: "Total number of DHCP errors by type",
			},
			[]string{"type"},
		),

		// Lease metrics
		ActiveLeases: promauto.NewGauge(
			prometheus.GaugeOpts{
				Name: "irondhcp_leases_active",
				Help: "Number of active DHCP leases",
			},
		),

		ExpiredLeases: promauto.NewGauge(
			prometheus.GaugeOpts{
				Name: "irondhcp_leases_expired",
				Help: "Number of expired DHCP leases",
			},
		),

		TotalLeases: promauto.NewCounter(
			prometheus.CounterOpts{
				Name: "irondhcp_leases_total",
				Help: "Total number of DHCP leases issued",
			},
		),

		LeaseRenewals: promauto.NewCounter(
			prometheus.CounterOpts{
				Name: "irondhcp_lease_renewals_total",
				Help: "Total number of lease renewals",
			},
		),

		LeaseReleases: promauto.NewCounter(
			prometheus.CounterOpts{
				Name: "irondhcp_lease_releases_total",
				Help: "Total number of lease releases",
			},
		),

		LeaseDeclines: promauto.NewCounter(
			prometheus.CounterOpts{
				Name: "irondhcp_lease_declines_total",
				Help: "Total number of lease declines (IP conflicts)",
			},
		),

		// IP allocation metrics
		IPAllocations: promauto.NewCounter(
			prometheus.CounterOpts{
				Name: "irondhcp_ip_allocations_total",
				Help: "Total number of IP address allocations",
			},
		),

		IPAllocationErrors: promauto.NewCounter(
			prometheus.CounterOpts{
				Name: "irondhcp_ip_allocation_errors_total",
				Help: "Total number of IP allocation errors",
			},
		),

		IPAllocationDuration: promauto.NewHistogram(
			prometheus.HistogramOpts{
				Name:    "irondhcp_ip_allocation_duration_seconds",
				Help:    "Duration of IP allocation operations",
				Buckets: prometheus.DefBuckets,
			},
		),

		// Cluster/HA metrics
		IPAllocationsPerServer: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "irondhcp_allocations_per_server_total",
				Help: "Total DHCP allocations by server ID and subnet",
			},
			[]string{"server_id", "subnet"},
		),

		AllocationRetries: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "irondhcp_allocation_retries",
				Help:    "Number of retries before successful allocation (indicates lock contention)",
				Buckets: []float64{0, 1, 2, 5, 10, 20, 50},
			},
			[]string{"server_id", "subnet"},
		),

		DatabaseLatency: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "irondhcp_database_latency_seconds",
				Help:    "Database query latency by operation type",
				Buckets: []float64{0.001, 0.005, 0.010, 0.025, 0.050, 0.100, 0.250, 0.500, 1.0},
			},
			[]string{"operation"},
		),

		// Reservation metrics
		StaticReservations: promauto.NewGauge(
			prometheus.GaugeOpts{
				Name: "irondhcp_reservations_total",
				Help: "Total number of static IP reservations",
			},
		),

		// Git sync metrics
		GitSyncs: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "irondhcp_git_syncs_total",
				Help: "Total number of git sync operations by status",
			},
			[]string{"status"}, // success, failed
		),

		GitSyncDuration: promauto.NewHistogram(
			prometheus.HistogramOpts{
				Name:    "irondhcp_git_sync_duration_seconds",
				Help:    "Duration of git sync operations",
				Buckets: prometheus.DefBuckets,
			},
		),

		GitSyncLastTimestamp: promauto.NewGauge(
			prometheus.GaugeOpts{
				Name: "irondhcp_git_sync_last_timestamp",
				Help: "Timestamp of the last successful git sync",
			},
		),

		// Database metrics
		DatabaseQueries: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "irondhcp_database_queries_total",
				Help: "Total number of database queries by operation",
			},
			[]string{"operation"}, // select, insert, update, delete
		),

		DatabaseErrors: promauto.NewCounter(
			prometheus.CounterOpts{
				Name: "irondhcp_database_errors_total",
				Help: "Total number of database errors",
			},
		),

		DatabaseConnections: promauto.NewGauge(
			prometheus.GaugeOpts{
				Name: "irondhcp_database_connections",
				Help: "Number of active database connections",
			},
		),
	}

	return m
}

// RecordDHCPRequest records a DHCP request
func (m *Metrics) RecordDHCPRequest(messageType string) {
	m.DHCPRequests.WithLabelValues(messageType).Inc()
}

// RecordDHCPResponse records a DHCP response
func (m *Metrics) RecordDHCPResponse(messageType string) {
	m.DHCPResponses.WithLabelValues(messageType).Inc()
}

// RecordDHCPError records a DHCP error
func (m *Metrics) RecordDHCPError(errorType string) {
	m.DHCPErrors.WithLabelValues(errorType).Inc()
}

// RecordIPAllocation records an IP allocation
func (m *Metrics) RecordIPAllocation(duration float64) {
	m.IPAllocations.Inc()
	m.IPAllocationDuration.Observe(duration)
}

// RecordIPAllocationError records an IP allocation error
func (m *Metrics) RecordIPAllocationError() {
	m.IPAllocationErrors.Inc()
}

// RecordLeaseRenewal records a lease renewal
func (m *Metrics) RecordLeaseRenewal() {
	m.LeaseRenewals.Inc()
}

// RecordLeaseRelease records a lease release
func (m *Metrics) RecordLeaseRelease() {
	m.LeaseReleases.Inc()
}

// RecordLeaseDecline records a lease decline
func (m *Metrics) RecordLeaseDecline() {
	m.LeaseDeclines.Inc()
}

// RecordGitSync records a git sync operation
func (m *Metrics) RecordGitSync(success bool, duration float64) {
	status := "success"
	if !success {
		status = "failed"
	}
	m.GitSyncs.WithLabelValues(status).Inc()
	m.GitSyncDuration.Observe(duration)

	if success {
		m.GitSyncLastTimestamp.SetToCurrentTime()
	}
}

// UpdateLeaseMetrics updates lease count metrics
func (m *Metrics) UpdateLeaseMetrics(active, expired int) {
	m.ActiveLeases.Set(float64(active))
	m.ExpiredLeases.Set(float64(expired))
}

// UpdateReservationMetrics updates reservation count metrics
func (m *Metrics) UpdateReservationMetrics(count int) {
	m.StaticReservations.Set(float64(count))
}

// RecordDatabaseQuery records a database query
func (m *Metrics) RecordDatabaseQuery(operation string) {
	m.DatabaseQueries.WithLabelValues(operation).Inc()
}

// RecordDatabaseError records a database error
func (m *Metrics) RecordDatabaseError() {
	m.DatabaseErrors.Inc()
}

// UpdateDatabaseConnections updates database connection count
func (m *Metrics) UpdateDatabaseConnections(count int) {
	m.DatabaseConnections.Set(float64(count))
}

// RecordServerAllocation records an IP allocation by a specific server
func (m *Metrics) RecordServerAllocation(serverID, subnet string) {
	if serverID != "" {
		m.IPAllocationsPerServer.WithLabelValues(serverID, subnet).Inc()
	}
}

// RecordAllocationRetries records the number of retries for an allocation
func (m *Metrics) RecordAllocationRetries(serverID, subnet string, retries float64) {
	if serverID != "" {
		m.AllocationRetries.WithLabelValues(serverID, subnet).Observe(retries)
	}
}

// RecordDatabaseLatency records database operation latency
func (m *Metrics) RecordDatabaseLatency(operation string, duration float64) {
	m.DatabaseLatency.WithLabelValues(operation).Observe(duration)
}
