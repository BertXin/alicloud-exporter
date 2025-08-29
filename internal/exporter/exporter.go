package exporter

import (
	"alicloud-exporter/internal/client"
	"alicloud-exporter/internal/collector"
	"alicloud-exporter/internal/config"
	"alicloud-exporter/internal/logger"
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// Exporter manages all service collectors and implements prometheus.Collector
type Exporter struct {
	client     *client.Client
	config     *config.Config
	logger     *logger.Logger
	collectors []collector.ServiceCollector
	mu         sync.RWMutex

	// Internal metrics
	up              prometheus.Gauge
	totalScrapes    prometheus.Counter
	scrapeErrors    prometheus.Counter
	scrapeDuration  prometheus.Histogram
	lastScrapeTime  prometheus.Gauge
	lastScrapeError prometheus.Gauge
}

// New creates a new Exporter with logger
func New(cfg *config.Config, log *logger.Logger) (*Exporter, error) {
	// Create Alicloud client
	client, err := client.NewClient(&cfg.Alicloud)
	if err != nil {
		return nil, fmt.Errorf("failed to create Alicloud client: %w", err)
	}

	exporter := &Exporter{
		client: client,
		config: cfg,
		logger: log,
		up: prometheus.NewGauge(prometheus.GaugeOpts{
			Name:        prometheus.BuildFQName(cfg.Prometheus.MetricPrefix, "", "up"),
			Help:        "Was the last scrape of Alicloud successful.",
			ConstLabels: cfg.Prometheus.GlobalLabels,
		}),
		totalScrapes: prometheus.NewCounter(prometheus.CounterOpts{
			Name:        prometheus.BuildFQName(cfg.Prometheus.MetricPrefix, "", "scrapes_total"),
			Help:        "Total number of times Alicloud was scraped for metrics.",
			ConstLabels: cfg.Prometheus.GlobalLabels,
		}),
		scrapeErrors: prometheus.NewCounter(prometheus.CounterOpts{
			Name:        prometheus.BuildFQName(cfg.Prometheus.MetricPrefix, "", "scrape_errors_total"),
			Help:        "Total number of times an error occurred scraping Alicloud.",
			ConstLabels: cfg.Prometheus.GlobalLabels,
		}),
		scrapeDuration: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name:        prometheus.BuildFQName(cfg.Prometheus.MetricPrefix, "", "scrape_duration_seconds"),
			Help:        "Time spent on scraping Alicloud.",
			ConstLabels: cfg.Prometheus.GlobalLabels,
			Buckets:     prometheus.DefBuckets,
		}),
		lastScrapeTime: prometheus.NewGauge(prometheus.GaugeOpts{
			Name:        prometheus.BuildFQName(cfg.Prometheus.MetricPrefix, "", "last_scrape_timestamp_seconds"),
			Help:        "Unix timestamp of the last scrape of Alicloud.",
			ConstLabels: cfg.Prometheus.GlobalLabels,
		}),
		lastScrapeError: prometheus.NewGauge(prometheus.GaugeOpts{
			Name:        prometheus.BuildFQName(cfg.Prometheus.MetricPrefix, "", "last_scrape_error"),
			Help:        "Whether the last scrape of Alicloud resulted in an error (1 for error, 0 for success).",
			ConstLabels: cfg.Prometheus.GlobalLabels,
		}),
	}

	// Initialize collectors
	if err := exporter.initCollectors(); err != nil {
		return nil, fmt.Errorf("failed to initialize collectors: %w", err)
	}

	return exporter, nil
}

// initCollectors initializes all service collectors based on configuration
func (e *Exporter) initCollectors() error {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.collectors = make([]collector.ServiceCollector, 0)

	// Initialize SLB collector
	if e.config.Services.SLB.Enabled {
		slbCollector := collector.NewSLBCollector(
			e.client,
			e.config.Services.SLB,
			e.config.Prometheus.GlobalLabels,
			e.config.Prometheus.MetricPrefix,
			e.logger,
		)
		e.collectors = append(e.collectors, slbCollector)
	}

	// Initialize Redis collector
	if e.config.Services.Redis.Enabled {
		redisCollector := collector.NewRedisCollector(
			e.client,
			e.config.Services.Redis,
			e.config.Prometheus.GlobalLabels,
			e.config.Prometheus.MetricPrefix,
			e.logger,
		)
		e.collectors = append(e.collectors, redisCollector)
	}

	// Initialize RDS collector
	if e.config.Services.RDS.Enabled {
		rdsCollector := collector.NewRDSCollector(
			e.client,
			e.config.Services.RDS,
			e.config.Prometheus.GlobalLabels,
			e.config.Prometheus.MetricPrefix,
			e.logger,
		)
		e.collectors = append(e.collectors, rdsCollector)
	}

	return nil
}

// Describe implements prometheus.Collector
func (e *Exporter) Describe(ch chan<- *prometheus.Desc) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	// Send internal metrics descriptors
	ch <- e.up.Desc()
	ch <- e.totalScrapes.Desc()
	ch <- e.scrapeErrors.Desc()
	ch <- e.scrapeDuration.Desc()
	ch <- e.lastScrapeTime.Desc()
	ch <- e.lastScrapeError.Desc()

	// Send collectors descriptors
	for _, collector := range e.collectors {
		collector.Describe(ch)
	}
}

// Collect implements prometheus.Collector
func (e *Exporter) Collect(ch chan<- prometheus.Metric) {
	start := time.Now()
	e.totalScrapes.Inc()

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	// Test client health
	if err := e.client.Health(ctx); err != nil {
		e.up.Set(0)
		e.scrapeErrors.Inc()
		e.lastScrapeError.Set(1)
		e.logger.Error("Alicloud client health check failed", "error", err)
	} else {
		e.up.Set(1)
		e.lastScrapeError.Set(0)
	}

	// Collect from all enabled collectors
	errorCount := 0
	var wg sync.WaitGroup
	errorCh := make(chan error, len(e.collectors))

	for _, col := range e.collectors {
		if !col.Enabled() {
			continue
		}

		wg.Add(1)
		go func(c collector.ServiceCollector) {
			defer wg.Done()
			if err := c.Collect(ctx, ch); err != nil {
				errorCh <- fmt.Errorf("collector %s failed: %w", c.Name(), err)
			}
		}(col)
	}

	wg.Wait()
	close(errorCh)

	// Count errors
	for err := range errorCh {
		errorCount++
		e.logger.Error("Collection error", "error", err)
	}

	if errorCount > 0 {
		e.scrapeErrors.Add(float64(errorCount))
		e.lastScrapeError.Set(1)
	} else {
		e.lastScrapeError.Set(0)
	}

	// Record scrape duration and time
	duration := time.Since(start)
	e.scrapeDuration.Observe(duration.Seconds())
	e.lastScrapeTime.Set(float64(time.Now().Unix()))

	// Send internal metrics
	ch <- e.up
	ch <- e.totalScrapes
	ch <- e.scrapeErrors
	ch <- e.scrapeDuration
	ch <- e.lastScrapeTime
	ch <- e.lastScrapeError
}

// Close closes the exporter and releases resources
func (e *Exporter) Close() error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.client != nil {
		e.client.Close()
	}

	return nil
}

// GetCollectors returns the list of active collectors
func (e *Exporter) GetCollectors() []collector.ServiceCollector {
	e.mu.RLock()
	defer e.mu.RUnlock()

	collectors := make([]collector.ServiceCollector, len(e.collectors))
	copy(collectors, e.collectors)
	return collectors
}

// GetConfig returns the exporter configuration
func (e *Exporter) GetConfig() *config.Config {
	return e.config
}
