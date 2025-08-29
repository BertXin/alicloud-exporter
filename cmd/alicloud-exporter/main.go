package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"alicloud-exporter/internal/config"
	"alicloud-exporter/internal/exporter"
	"alicloud-exporter/internal/logger"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/spf13/cobra"
)

var (
	version   = "dev"
	buildDate = "unknown"
	commitSHA = "unknown"
)

var (
	configFile  string
	logLevel    string
	logFormat   string
	showVersion bool
)

func main() {
	rootCmd := &cobra.Command{
		Use:   "alicloud-exporter",
		Short: "Prometheus exporter for Alicloud services",
		Long: `A Prometheus exporter that collects metrics from Alicloud services including:
- SLB (Server Load Balancer)
- Redis (KVStore)
- RDS (Relational Database Service)

The exporter provides standardized Prometheus metrics with proper labeling and error handling.`,
		RunE: runExporter,
	}

	// Add flags
	rootCmd.Flags().StringVarP(&configFile, "config", "c", "", "Path to configuration file")
	rootCmd.Flags().StringVar(&logLevel, "log-level", "info", "Log level (debug, info, warn, error)")
	rootCmd.Flags().StringVar(&logFormat, "log-format", "json", "Log format (json, text)")
	rootCmd.Flags().BoolVarP(&showVersion, "version", "v", false, "Show version information")

	// Add version command
	versionCmd := &cobra.Command{
		Use:   "version",
		Short: "Show version information",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("alicloud-exporter\n")
			fmt.Printf("  Version: %s\n", version)
			fmt.Printf("  Build Date: %s\n", buildDate)
			fmt.Printf("  Commit SHA: %s\n", commitSHA)
		},
	}
	rootCmd.AddCommand(versionCmd)

	// Add config validation command
	validateCmd := &cobra.Command{
		Use:   "validate",
		Short: "Validate configuration file",
		RunE:  validateConfig,
	}
	validateCmd.Flags().StringVarP(&configFile, "config", "c", "", "Path to configuration file")
	rootCmd.AddCommand(validateCmd)

	// Add metrics list command
	metricsCmd := &cobra.Command{
		Use:   "metrics",
		Short: "List available metrics for each service",
		Run:   listMetrics,
	}
	rootCmd.AddCommand(metricsCmd)

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func runExporter(cmd *cobra.Command, args []string) error {
	if showVersion {
		fmt.Printf("alicloud-exporter %s (built %s, commit %s)\n", version, buildDate, commitSHA)
		return nil
	}

	// Load configuration
	cfg, err := config.Load(configFile)
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	// Override log settings from command line if provided
	if cmd.Flags().Changed("log-level") {
		cfg.Server.LogLevel = logLevel
	}
	if cmd.Flags().Changed("log-format") {
		cfg.Server.LogFormat = logFormat
	}

	// Initialize logger
	log := logger.New(cfg.Server.LogLevel, cfg.Server.LogFormat)
	log.WithFields(map[string]interface{}{
		"version":    version,
		"build_date": buildDate,
		"commit_sha": commitSHA,
	}).Info("Starting alicloud-exporter")

	// Create exporter
	exp, err := exporter.New(cfg, log)
	if err != nil {
		return fmt.Errorf("failed to create exporter: %w", err)
	}
	defer exp.Close()

	// Register exporter with Prometheus
	registry := prometheus.NewRegistry()
	registry.MustRegister(exp)

	// Optionally register Go and process metrics
	if cfg.Prometheus.IncludeGoMetrics {
		registry.MustRegister(prometheus.NewGoCollector())
	}
	if cfg.Prometheus.IncludeProcessMetrics {
		registry.MustRegister(prometheus.NewProcessCollector(prometheus.ProcessCollectorOpts{}))
	}

	// Setup HTTP server
	mux := http.NewServeMux()
	mux.Handle(cfg.Server.MetricsPath, promhttp.HandlerFor(registry, promhttp.HandlerOpts{
		ErrorLog:      log.Logger,
		ErrorHandling: promhttp.ContinueOnError,
	}))

	// Add health check endpoint
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, `{"status":"healthy","version":"%s"}`, version)
	})

	// Add root endpoint with information
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprintf(w, `<html>
<head><title>Alicloud Exporter</title></head>
<body>
<h1>Alicloud Exporter</h1>
<p><a href="%s">Metrics</a></p>
<p><a href="/health">Health</a></p>
<p>Version: %s</p>
</body>
</html>`, cfg.Server.MetricsPath, version)
	})

	server := &http.Server{
		Addr:         cfg.Server.ListenAddress,
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Start server in goroutine
	go func() {
		log.WithField("address", cfg.Server.ListenAddress).Info("Starting HTTP server")
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.WithError(err).Fatal("HTTP server failed")
		}
	}()

	// Wait for interrupt signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	sig := <-sigChan

	log.WithField("signal", sig.String()).Info("Received shutdown signal")

	// Graceful shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		log.WithError(err).Error("Server shutdown failed")
		return err
	}

	log.Info("Server shutdown completed")
	return nil
}

func validateConfig(cmd *cobra.Command, args []string) error {
	if configFile == "" {
		return fmt.Errorf("config file is required")
	}

	cfg, err := config.Load(configFile)
	if err != nil {
		return fmt.Errorf("configuration validation failed: %w", err)
	}

	fmt.Printf("Configuration file '%s' is valid\n", configFile)
	fmt.Printf("Services enabled:\n")
	if cfg.Services.SLB.Enabled {
		fmt.Printf("  - SLB: %d metrics\n", len(cfg.Services.SLB.Metrics))
	}
	if cfg.Services.Redis.Enabled {
		fmt.Printf("  - Redis: %d metrics\n", len(cfg.Services.Redis.Metrics))
	}
	if cfg.Services.RDS.Enabled {
		fmt.Printf("  - RDS: %d metrics\n", len(cfg.Services.RDS.Metrics))
	}

	return nil
}

func listMetrics(cmd *cobra.Command, args []string) {
	fmt.Println("Available metrics by service:")
	fmt.Println()

	fmt.Println("SLB (Server Load Balancer):")
	for _, metric := range getSLBMetrics() {
		fmt.Printf("  - %s\n", metric)
	}
	fmt.Println()

	fmt.Println("Redis (KVStore):")
	for _, metric := range getRedisMetrics() {
		fmt.Printf("  - %s\n", metric)
	}
	fmt.Println()

	fmt.Println("RDS (Relational Database Service):")
	for _, metric := range getRDSMetrics() {
		fmt.Printf("  - %s\n", metric)
	}
}

// Metric lists (simplified versions for CLI)
func getSLBMetrics() []string {
	return []string{
		"ActiveConnection", "NewConnection", "DropConnection", "InactiveConnection",
		"MaxConnection", "Qps", "Rt", "StatusCode2xx", "StatusCode3xx",
		"StatusCode4xx", "StatusCode5xx", "TrafficRXNew", "TrafficTXNew",
		"HeathyServerCount", "UnhealthyServerCount",
	}
}

func getRedisMetrics() []string {
	return []string{
		"ConnectionUsage", "CpuUsage", "MemoryUsage", "UsedMemory",
		"UsedConnection", "UsedQPS", "IntranetIn", "IntranetOut",
		"IntranetInRatio", "IntranetOutRatio", "FailedCount",
	}
}

func getRDSMetrics() []string {
	return []string{
		"ConnectionUsage", "CpuUsage", "DiskUsage", "IOPSUsage",
		"MemoryUsage", "MySQL_ActiveSessions", "MySQL_QPS", "MySQL_TPS",
		"MySQL_NetworkInNew", "MySQL_NetworkOutNew", "MySQL_IbufDirtyRatio",
		"MySQL_IbufUseRatio", "MySQL_InnoDBDataRead", "MySQL_InnoDBDataWritten",
	}
}
