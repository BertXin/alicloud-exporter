package collector

import (
	"context"
	"fmt"
	"time"

	"alicloud-exporter/internal/client"
	"alicloud-exporter/internal/config"
	"alicloud-exporter/internal/logger"
	"github.com/prometheus/client_golang/prometheus"
)

// RedisCollector collects metrics from Alicloud Redis (KVStore)
type RedisCollector struct {
	*BaseCollector
}

// NewRedisCollector creates a new Redis collector
func NewRedisCollector(
	client *client.Client,
	config config.ServiceConfig,
	globalLabels map[string]string,
	metricPrefix string,
	log *logger.Logger,
) *RedisCollector {
	baseCollector := NewBaseCollector(
		client,
		config,
		"redis",
		globalLabels,
		metricPrefix,
		log,
	)

	return &RedisCollector{
		BaseCollector: baseCollector,
	}
}

// Collect implements the ServiceCollector interface
func (c *RedisCollector) Collect(ctx context.Context, ch chan<- prometheus.Metric) error {
	if !c.Enabled() {
		return nil
	}

	c.logger.Debug("Starting Redis metrics collection")
	start := time.Now()
	defer func() {
		c.RecordScrapeDuration(time.Since(start))
		c.SetLastScrapeTime(time.Now())
	}()

	// Send internal metrics
	ch <- c.scrapeErrors
	ch <- c.scrapeDuration

	errorCount := 0
	for _, metricName := range c.config.Metrics {
		if err := c.CollectMetric(ctx, metricName, ch); err != nil {
			errorCount++
			// Log error but continue with other metrics
			c.logger.WithField("metric", metricName).WithError(err).Error("Error collecting Redis metric")
		}
	}

	if errorCount > 0 {
		c.RecordScrapeError()
		return fmt.Errorf("failed to collect %d Redis metrics", errorCount)
	}

	return nil
}

// GetRedisMetrics returns the list of available Redis metrics
func GetRedisMetrics() []string {
	return []string{
		"ConnectionUsage",
		"CpuUsage",
		"MemoryUsage",
		"UsedMemory",
		"UsedConnection",
		"UsedQPS",
		"IntranetIn",
		"IntranetOut",
		"IntranetInRatio",
		"IntranetOutRatio",
		"FailedCount",
		"AvgRt",
		"MaxRt",
		"ExpiredKeys",
		"EvictedKeys",
		"HitRate",
		"Keys",
		"Expires",
		"ConnectedClients",
		"BlockedClients",
		"TotalCommandsProcessed",
		"InstantaneousOpsPerSec",
		"KeyspaceHits",
		"KeyspaceMisses",
		"PubsubChannels",
		"PubsubPatterns",
		"LatestForkUsec",
		"RdbChangesSinceLastSave",
		"SyncFull",
		"SyncPartialErr",
		"SyncPartialOk",
		"RejectedConnections",
		"SlowLogLen",
	}
}
