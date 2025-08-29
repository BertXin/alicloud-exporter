package collector

import (
	"alicloud-exporter/internal/client"
	"alicloud-exporter/internal/config"
	"alicloud-exporter/internal/logger"
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// RDSCollector collects metrics from Alicloud RDS (Relational Database Service)
type RDSCollector struct {
	*BaseCollector
}

// NewRDSCollector creates a new RDS collector
func NewRDSCollector(
	client *client.Client,
	config config.ServiceConfig,
	globalLabels map[string]string,
	metricPrefix string,
	log *logger.Logger,
) *RDSCollector {
	baseCollector := NewBaseCollector(
		client,
		config,
		"rds",
		globalLabels,
		metricPrefix,
		log,
	)

	return &RDSCollector{
		BaseCollector: baseCollector,
	}
}

// Collect implements the ServiceCollector interface
func (c *RDSCollector) Collect(ctx context.Context, ch chan<- prometheus.Metric) error {
	if !c.Enabled() {
		return nil
	}

	c.logger.Debug("Starting RDS metrics collection")
	start := time.Now()
	defer func() {
		c.RecordScrapeDuration(time.Since(start))
		c.SetLastScrapeTime(time.Now())
	}()

	// Send internal metrics
	ch <- c.scrapeErrors
	ch <- c.scrapeDuration

	// Use concurrent collection for better performance
	errorCount := c.collectMetricsConcurrently(ctx, ch)

	if errorCount > 0 {
		c.RecordScrapeError()
		return fmt.Errorf("failed to collect %d RDS metrics", errorCount)
	}

	return nil
}

// collectMetricsConcurrently collects metrics concurrently for better performance
func (c *RDSCollector) collectMetricsConcurrently(ctx context.Context, ch chan<- prometheus.Metric) int {
	var wg sync.WaitGroup
	errorCh := make(chan error, len(c.config.Metrics))

	// Limit concurrent goroutines to avoid overwhelming the API
	maxConcurrent := 10
	semaphore := make(chan struct{}, maxConcurrent)

	for _, metricName := range c.config.Metrics {
		wg.Add(1)
		go func(metric string) {
			defer wg.Done()
			semaphore <- struct{}{} // Acquire semaphore
			defer func() { <-semaphore }() // Release semaphore

			if err := c.CollectMetric(ctx, metric, ch); err != nil {
				errorCh <- fmt.Errorf("metric %s: %w", metric, err)
				c.logger.WithField("metric", metric).WithError(err).Error("Error collecting RDS metric")
			}
		}(metricName)
	}

	wg.Wait()
	close(errorCh)

	// Count errors
	errorCount := 0
	for range errorCh {
		errorCount++
	}

	return errorCount
}

// GetRDSMetrics returns the list of available RDS metrics
func GetRDSMetrics() []string {
	return []string{
		"ConnectionUsage",
		"CpuUsage",
		"DiskUsage",
		"IOPSUsage",
		"MemoryUsage",
		"MySQL_ActiveSessions",
		"MySQL_QPS",
		"MySQL_TPS",
		"MySQL_NetworkInNew",
		"MySQL_NetworkOutNew",
		"MySQL_IbufDirtyRatio",
		"MySQL_IbufUseRatio",
		"MySQL_InnoDBDataRead",
		"MySQL_InnoDBDataWritten",
		"MySQL_ComDelete",
		"MySQL_ComInsert",
		"MySQL_ComInsertSelect",
		"MySQL_ComReplace",
		"MySQL_ComReplaceSelect",
		"MySQL_ComSelect",
		"MySQL_ComUpdate",
		"MySQL_TempDiskTableCreates",
		"MySQL_InnoDBRowUpdate",
		"MySQL_InnoDBRowInsert",
		"MySQL_InnoDBRowDelete",
		"MySQL_InnoDBRowRead",
		"MySQL_InnoDBLogFsync",
		"MySQL_InnoDBLogWrites",
		"MySQL_InnoDBLogWriteRequests",
		"MySQL_SlowQueries",
		"MySQL_ThreadsConnected",
		"MySQL_ThreadsRunning",
		"MySQL_CreatedTmpDiskTables",
		"MySQL_CreatedTmpTables",
		"MySQL_OpenTables",
		"MySQL_TableLocksWaited",
		"MySQL_TableLocksImmediate",
		"MySQL_InnoDBBufferPoolReads",
		"MySQL_InnoDBBufferPoolReadRequests",
		"MySQL_InnoDBBufferPoolUtilization",
		"MySQL_InnoDBDataReads",
		"MySQL_InnoDBDataWrites",
		"MySQL_InnoDBOsLogFsyncs",
		"MySQL_InnoDBOsLogWrites",
		"MySQL_InnoDBLogWaits",
		"MySQL_BinlogDiskUsage",
		"MySQL_RelayLogDiskUsage",
		"MySQL_TmpDiskUsage",
		"MySQL_DataDiskUsage",
		"MySQL_LogDiskUsage",
		"MySQL_OtherDiskUsage",
		"MySQL_SlaveIORunning",
		"MySQL_SlaveSQLRunning",
		"MySQL_SecondsBehindMaster",
		"MySQL_MasterBinlogSize",
		"MySQL_SlaveBinlogSize",
	}
}
