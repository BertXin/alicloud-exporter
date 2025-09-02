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

	// Get tags and regions for all instances
	var tagsMap map[string]map[string]string
	var regionsMap map[string]string
	if len(instanceIDs) > 0 {
		tagsMap, regionsMap, err = c.client.GetSLBInstanceTagsWithRegion(ctx, instanceIDs)
		if err != nil {
			c.logger.WithError(err).Warn("Failed to get SLB instance tags, continuing without tags")
			tagsMap = make(map[string]map[string]string)
			regionsMap = make(map[string]string)
		}
	} else {
		tagsMap = make(map[string]map[string]string)
		regionsMap = make(map[string]string)
	}

	// Only keep Team, Group, Name tags and add region
	tagKeys := []string{"Team", "Group", "Name"}

	// Create descriptor with only the required tags plus region
	labels := []string{"instance_id", "protocol", "port", "vip", "region"}
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

		labelValues := c.buildDynamicSLBLabelValues(data, tagsMap[data.InstanceID], regionsMap[data.InstanceID], tagKeys)

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

// buildSLBLabelValues builds label values for SLB metrics with tags and region
func (c *SLBCollector) buildSLBLabelValues(data MetricData, tags map[string]string, region string) []string {
	switch c.serviceName {
	case "slb":
		// Use instance-specific region if available, otherwise fall back to client region
		instanceRegion := region
		if instanceRegion == "" {
			instanceRegion = c.client.GetRegion()
		}
		// Get tag values, use empty string if not found (support both uppercase and lowercase)
		team := ""
		group := ""
		name := ""
		if tags != nil {
			// Try both uppercase and lowercase for team
			if val, ok := tags["team"]; ok {
				team = val
			} 
			// Try both uppercase and lowercase for group
			if val, ok := tags["Group"]; ok {
				group = val
			} 
			// Try both uppercase and lowercase for name
			if val, ok := tags["Name"]; ok {
				name = val
			}
		}
		return []string{data.InstanceID, data.Protocol, data.Port, data.Vip, instanceRegion, team, group, name}
	default:
		// Fallback to base implementation with region
		instanceRegion := region
		if instanceRegion == "" {
			instanceRegion = c.client.GetRegion()
		}
		// For non-SLB services, use base buildLabelValues but this shouldn't happen in SLB collector
		return c.BaseCollector.buildLabelValues(data)
	}
}

// buildDynamicSLBLabelValues builds label values for SLB metrics with only Team, Group, Name tags and region
func (c *SLBCollector) buildDynamicSLBLabelValues(data MetricData, tags map[string]string, region string, tagKeys []string) []string {
	// Use instance-specific region if available, otherwise fall back to client region
	instanceRegion := region
	if instanceRegion == "" {
		instanceRegion = c.client.GetRegion()
	}

	// Start with basic SLB labels including region
	labelValues := []string{data.InstanceID, data.Protocol, data.Port, data.Vip, instanceRegion}

	// Add only Team, Group, Name tag values (support both uppercase and lowercase)
	team := ""
	group := ""
	name := ""
	if tags != nil {
		// Try both uppercase and lowercase for team
		if val, ok := tags["team"]; ok {
			team = val
		} 
		// Try both uppercase and lowercase for group
		if val, ok := tags["Group"]; ok {
			group = val
		} 
		// Try both uppercase and lowercase for name
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
