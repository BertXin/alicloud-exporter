package collector

import (
	"alicloud-exporter/internal/client"
	"alicloud-exporter/internal/config"
	"alicloud-exporter/internal/logger"
	"context"
	"encoding/json"
	"fmt"
	"github.com/prometheus/client_golang/prometheus"
	"sync"
	"time"
)

// MetricData represents the structure of metric data from Alicloud CMS
type MetricData struct {
	Timestamp  int64   `json:"timestamp"`
	UserID     string  `json:"userId,omitempty"`
	InstanceID string  `json:"instanceId"`
	Device     string  `json:"device,omitempty"`
	Port       string  `json:"port,omitempty"`
	Protocol   string  `json:"protocol,omitempty"`
	Vip        string  `json:"vip,omitempty"`
	State      string  `json:"state,omitempty"`
	Diskname   string  `json:"diskname,omitempty"`
	Sum        float64 `json:"Sum"`
	Maximum    float64 `json:"Maximum"`
	Average    float64 `json:"Average"`
	Minimum    float64 `json:"Minimum"`
}

// ServiceCollector defines the interface for service-specific collectors
type ServiceCollector interface {
	// Describe sends the descriptors of metrics collected by this collector
	Describe(ch chan<- *prometheus.Desc)

	// Collect fetches metrics from Alicloud and sends them to the channel
	Collect(ctx context.Context, ch chan<- prometheus.Metric) error

	// Name returns the name of the service
	Name() string

	// Enabled returns whether this collector is enabled
	Enabled() bool
}

// BaseCollector provides common functionality for all collectors
type BaseCollector struct {
	client         *client.Client
	config         config.ServiceConfig
	logger         *logger.Logger
	serviceName    string
	metricDescs    map[string]*prometheus.Desc
	globalLabels   map[string]string
	metricPrefix   string
	mu             sync.RWMutex
	lastScrape     time.Time
	scrapeErrors   prometheus.Counter
	scrapeDuration prometheus.Histogram
}

// NewBaseCollector creates a new base collector
func NewBaseCollector(
	client *client.Client,
	config config.ServiceConfig,
	serviceName string,
	globalLabels map[string]string,
	metricPrefix string,
	log *logger.Logger,
) *BaseCollector {
	bc := &BaseCollector{
		client:       client,
		config:       config,
		logger:       log,
		serviceName:  serviceName,
		metricDescs:  make(map[string]*prometheus.Desc),
		globalLabels: globalLabels,
		metricPrefix: metricPrefix,
		scrapeErrors: prometheus.NewCounter(prometheus.CounterOpts{
			Name:        prometheus.BuildFQName(metricPrefix, serviceName, "scrape_errors_total"),
			Help:        fmt.Sprintf("Total number of scrape errors for %s service", serviceName),
			ConstLabels: globalLabels,
		}),
		scrapeDuration: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name:        prometheus.BuildFQName(metricPrefix, serviceName, "scrape_duration_seconds"),
			Help:        fmt.Sprintf("Duration of scrape for %s service", serviceName),
			ConstLabels: globalLabels,
			Buckets:     prometheus.DefBuckets,
		}),
	}

	// Initialize metric descriptors
	bc.initMetricDescriptors()

	return bc
}

// initMetricDescriptors initializes Prometheus metric descriptors
func (bc *BaseCollector) initMetricDescriptors() {
	for _, metricName := range bc.config.Metrics {
		// Base labels that are always present
		labels := []string{"instance_id"}

		// Add service-specific labels (only non-empty ones will be used)
		switch bc.serviceName {
		case "slb":
			// For SLB, we'll create descriptors dynamically with tags
			labels = append(labels, "protocol", "port", "vip")
		case "redis":
			// Redis - only add labels that might have values
			// We'll filter empty values in buildLabelValues
		case "rds":
			// RDS - only add labels that might have values
			// We'll filter empty values in buildLabelValues
		}

		bc.metricDescs[metricName] = prometheus.NewDesc(
			prometheus.BuildFQName(bc.metricPrefix, bc.serviceName, metricName),
			fmt.Sprintf("%s metric from Alicloud CMS", metricName),
			labels,
			bc.globalLabels,
		)
	}
}

// Name returns the service name
func (bc *BaseCollector) Name() string {
	return bc.serviceName
}

// Enabled returns whether this collector is enabled
func (bc *BaseCollector) Enabled() bool {
	return bc.config.Enabled
}

// Describe sends metric descriptors to the channel
func (bc *BaseCollector) Describe(ch chan<- *prometheus.Desc) {
	bc.mu.RLock()
	defer bc.mu.RUnlock()

	for _, desc := range bc.metricDescs {
		ch <- desc
	}

	// Send internal metrics descriptors
	ch <- bc.scrapeErrors.Desc()
	ch <- bc.scrapeDuration.Desc()
}

// CollectMetric collects a single metric from Alicloud CMS
func (bc *BaseCollector) CollectMetric(ctx context.Context, metricName string, ch chan<- prometheus.Metric) error {
	response, err := bc.client.GetMetricData(ctx, bc.config.Namespace, metricName)
	if err != nil {
		return fmt.Errorf("failed to get metric %s: %w", metricName, err)
	}

	if response.Datapoints == "" {
		return nil // No data available
	}

	var metricData []MetricData
	if err := json.Unmarshal([]byte(response.Datapoints), &metricData); err != nil {
		return fmt.Errorf("failed to unmarshal metric data for %s: %w", metricName, err)
	}

	desc, exists := bc.metricDescs[metricName]
	if !exists {
		return fmt.Errorf("metric descriptor not found for %s", metricName)
	}

	for _, data := range metricData {
		value := data.Average
		if value == 0 {
			value = data.Maximum
		}
		if value == 0 {
			value = data.Sum
		}

		labelValues := bc.buildLabelValues(data)

		metric, err := prometheus.NewConstMetric(
			desc,
			prometheus.GaugeValue,
			value,
			labelValues...,
		)
		if err != nil {
			return fmt.Errorf("failed to create metric for %s: %w", metricName, err)
		}

		ch <- metric
	}

	return nil
}

// buildLabelValues builds label values based on service type and metric data
func (bc *BaseCollector) buildLabelValues(data MetricData) []string {
	switch bc.serviceName {
	case "slb":
		return []string{
			data.InstanceID,
			data.Protocol,
			data.Port,
			data.Vip,
		}
	case "redis", "rds":
		// Only return instance_id for redis and rds to avoid empty labels
		return []string{data.InstanceID}
	default:
		return []string{data.InstanceID}
	}
}

// RecordScrapeError records a scrape error
func (bc *BaseCollector) RecordScrapeError() {
	bc.scrapeErrors.Inc()
}

// RecordScrapeDuration records the duration of a scrape
func (bc *BaseCollector) RecordScrapeDuration(duration time.Duration) {
	bc.scrapeDuration.Observe(duration.Seconds())
}

// GetLastScrapeTime returns the last scrape time
func (bc *BaseCollector) GetLastScrapeTime() time.Time {
	bc.mu.RLock()
	defer bc.mu.RUnlock()
	return bc.lastScrape
}

// SetLastScrapeTime sets the last scrape time
func (bc *BaseCollector) SetLastScrapeTime(t time.Time) {
	bc.mu.Lock()
	defer bc.mu.Unlock()
	bc.lastScrape = t
}
