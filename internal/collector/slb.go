package collector

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"alicloud-exporter/internal/client"
	"alicloud-exporter/internal/config"
	"alicloud-exporter/internal/logger"
	"github.com/prometheus/client_golang/prometheus"
)

// SLBCollector collects metrics from Alicloud SLB (Server Load Balancer)
type SLBCollector struct {
	*BaseCollector
}

// NewSLBCollector creates a new SLB collector
func NewSLBCollector(
	client *client.Client,
	config config.ServiceConfig,
	globalLabels map[string]string,
	metricPrefix string,
	log *logger.Logger,
) *SLBCollector {
	baseCollector := NewBaseCollector(
		client,
		config,
		"slb",
		globalLabels,
		metricPrefix,
		log,
	)

	return &SLBCollector{
		BaseCollector: baseCollector,
	}
}

// Collect implements the ServiceCollector interface
func (c *SLBCollector) Collect(ctx context.Context, ch chan<- prometheus.Metric) error {
	if !c.Enabled() {
		return nil
	}

	c.logger.Debug("Starting SLB metrics collection")
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
		if err := c.CollectSLBMetric(ctx, metricName, ch); err != nil {
			errorCount++
			// Log error but continue with other metrics
			c.logger.WithField("metric", metricName).WithError(err).Error("Error collecting SLB metric")
		}
	}

	if errorCount > 0 {
		c.RecordScrapeError()
		return fmt.Errorf("failed to collect %d SLB metrics", errorCount)
	}

	return nil
}

// CollectSLBMetric collects a specific SLB metric with dynamic tag enrichment
func (c *SLBCollector) CollectSLBMetric(ctx context.Context, metricName string, ch chan<- prometheus.Metric) error {
	response, err := c.client.GetMetricData(ctx, c.config.Namespace, metricName)
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

	// Extract unique instance IDs efficiently
	instanceSet := make(map[string]bool)
	for _, data := range metricData {
		if data.InstanceID != "" {
			instanceSet[data.InstanceID] = true
		}
	}

	// Convert set to slice
	instanceIDs := make([]string, 0, len(instanceSet))
	for instanceID := range instanceSet {
		instanceIDs = append(instanceIDs, instanceID)
	}

	// Get tags for all instances
	var tagsMap map[string]map[string]string
	if len(instanceIDs) > 0 {
		tagsMap, err = c.client.GetSLBInstanceTags(ctx, instanceIDs)
		if err != nil {
			c.logger.WithError(err).Warn("Failed to get SLB instance tags, continuing without tags")
			tagsMap = make(map[string]map[string]string)
		}
	} else {
		tagsMap = make(map[string]map[string]string)
	}

	// Only keep Team, Group, Name tags
	tagKeys := []string{"Team", "Group", "Name"}

	// Create descriptor with only the required tags
	labels := []string{"instance_id", "protocol", "port", "vip"}
	labels = append(labels, tagKeys...)

	dynamicDesc := prometheus.NewDesc(
		prometheus.BuildFQName(c.metricPrefix, c.serviceName, metricName),
		fmt.Sprintf("%s metric from Alicloud CMS", metricName),
		labels,
		c.globalLabels,
	)

	for _, data := range metricData {
		value := data.Average
		if value == 0 {
			value = data.Maximum
		}
		if value == 0 {
			value = data.Sum
		}

		labelValues := c.buildDynamicSLBLabelValues(data, tagsMap[data.InstanceID], tagKeys)

		metric, err := prometheus.NewConstMetric(
			dynamicDesc,
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

// buildSLBLabelValues builds label values for SLB metrics with tags
func (c *SLBCollector) buildSLBLabelValues(data MetricData, tags map[string]string) []string {
	switch c.serviceName {
	case "slb":
		// Get tag values, use empty string if not found
		team := ""
		group := ""
		name := ""
		if tags != nil {
			if val, ok := tags["Team"]; ok {
				team = val
			}
			if val, ok := tags["Group"]; ok {
				group = val
			}
			if val, ok := tags["Name"]; ok {
				name = val
			}
		}
		return []string{data.InstanceID, data.Protocol, data.Port, data.Vip, team, group, name}
	default:
		// Fallback to base implementation
		return c.buildLabelValues(data)
	}
}

// buildDynamicSLBLabelValues builds label values for SLB metrics with only Team, Group, Name tags
func (c *SLBCollector) buildDynamicSLBLabelValues(data MetricData, tags map[string]string, tagKeys []string) []string {
	// Start with basic SLB labels
	labelValues := []string{data.InstanceID, data.Protocol, data.Port, data.Vip}

	// Add only Team, Group, Name tag values
	team := ""
	group := ""
	name := ""
	if tags != nil {
		if val, ok := tags["team"]; ok {
			team = val
		}
		if val, ok := tags["Group"]; ok {
			group = val
		}
		if val, ok := tags["Name"]; ok {
			name = val
		}
	}

	labelValues = append(labelValues, team, group, name)
	return labelValues
}

// GetSLBMetrics returns the list of available SLB metrics
func GetSLBMetrics() []string {
	return []string{
		"ActiveConnection",
		"NewConnection",
		"DropConnection",
		"InactiveConnection",
		"MaxConnection",
		"Qps",
		"Rt",
		"StatusCode2xx",
		"StatusCode3xx",
		"StatusCode4xx",
		"StatusCode5xx",
		"StatusCodeOther",
		"TrafficRXNew",
		"TrafficTXNew",
		"HeathyServerCount",
		"UnhealthyServerCount",
		"DropPackerRX",
		"DropPackerTX",
		"DropTrafficRX",
		"DropTrafficTX",
		"PacketRX",
		"PacketTX",
		"UpstreamCode4xx",
		"UpstreamCode5xx",
		"UpstreamRt",
		"InstanceActiveConnection",
		"InstanceDropConnection",
		"InstanceDropPacketRX",
		"InstanceDropPacketTX",
		"InstanceDropTrafficRX",
		"InstanceDropTrafficTX",
		"InstanceInactiveConnection",
		"InstanceMaxConnection",
		"InstanceMaxConnectionUtilization",
		"InstanceNewConnection",
		"InstanceNewConnectionUtilization",
		"InstancePacketRX",
		"InstancePacketTX",
		"InstanceQps",
		"InstanceQpsUtilization",
		"InstanceRt",
		"InstanceStatusCode2xx",
		"InstanceStatusCode3xx",
		"InstanceStatusCode4xx",
		"InstanceStatusCode5xx",
		"InstanceStatusCodeOther",
		"InstanceTrafficRX",
		"InstanceTrafficTX",
		"InstanceUpstreamCode4xx",
		"InstanceUpstreamCode5xx",
		"InstanceUpstreamRt",
		"GroupTotalTrafficRX",
		"GroupTotalTrafficTX",
	}
}
