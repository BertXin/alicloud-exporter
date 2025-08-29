package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"alicloud-exporter/internal/client"
	"alicloud-exporter/internal/config"
)

func main() {
	// Load configuration
	cfg := &config.Config{
		Alicloud: config.AlicloudConfig{
			AccessKeyID:     "",
			AccessKeySecret: "",
			Region:          "us-east-1",
			RateLimit: config.RateLimitConfig{
				RequestsPerSecond: 50,
				Burst:             100,
			},
		},
	}

	// Create client
	client, err := client.NewClient(&cfg.Alicloud)
	if err != nil {
		log.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	// Test metrics
	metrics := []string{
		"ConnectionUsage",
		"CpuUsage",
		"DiskUsage",
		"IOPSUsage",
		"MemoryUsage",
	}

	namespace := "acs_rds_dashboard"
	ctx := context.Background()

	fmt.Println("Performance Test - Sequential vs Concurrent")
	fmt.Println("===========================================")

	// Test 1: Sequential requests (old way)
	fmt.Println("\n1. Sequential Requests:")
	start := time.Now()
	for _, metric := range metrics {
		_, err := client.GetMetricData(ctx, namespace, metric)
		if err != nil {
			fmt.Printf("Error getting %s: %v\n", metric, err)
			continue
		}
		fmt.Printf("  ✓ %s\n", metric)
	}
	sequentialTime := time.Since(start)
	fmt.Printf("Sequential time: %v\n", sequentialTime)

	// Test 2: Concurrent requests (new way)
	fmt.Println("\n2. Concurrent Requests:")
	start = time.Now()
	resultCh := make(chan string, len(metrics))
	errorCh := make(chan error, len(metrics))

	for _, metric := range metrics {
		go func(m string) {
			_, err := client.GetMetricData(ctx, namespace, m)
			if err != nil {
				errorCh <- fmt.Errorf("%s: %v", m, err)
				return
			}
			resultCh <- m
		}(metric)
	}

	// Collect results
	for i := 0; i < len(metrics); i++ {
		select {
		case result := <-resultCh:
			fmt.Printf("  ✓ %s\n", result)
		case err := <-errorCh:
			fmt.Printf("  ✗ Error: %v\n", err)
		}
	}
	concurrentTime := time.Since(start)
	fmt.Printf("Concurrent time: %v\n", concurrentTime)

	// Test 3: Cache effectiveness
	fmt.Println("\n3. Cache Test (second request should be faster):")
	start = time.Now()
	_, err = client.GetMetricData(ctx, namespace, "CpuUsage")
	if err != nil {
		fmt.Printf("Error: %v\n", err)
	} else {
		fmt.Printf("Cached request time: %v\n", time.Since(start))
	}

	// Summary
	fmt.Println("\n===========================================")
	fmt.Printf("Performance Improvement: %.2fx faster\n", float64(sequentialTime)/float64(concurrentTime))
	fmt.Printf("Time saved: %v\n", sequentialTime-concurrentTime)
}
