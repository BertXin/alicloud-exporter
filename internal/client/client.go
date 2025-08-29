package client

import (
	"alicloud-exporter/internal/config"
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/aliyun/alibaba-cloud-sdk-go/services/cms"
	"github.com/aliyun/alibaba-cloud-sdk-go/services/slb"
)

// CacheEntry represents a cached metric data entry
type CacheEntry struct {
	Data      *cms.DescribeMetricLastResponse
	Timestamp time.Time
	TTL       time.Duration
}

// IsExpired checks if the cache entry has expired
func (ce *CacheEntry) IsExpired() bool {
	return time.Since(ce.Timestamp) > ce.TTL
}

// TagCache implements an in-memory cache for SLB tags
type TagCache struct {
	cache map[string]map[string]string
	mu    sync.RWMutex
	ttl   time.Duration
}

// NewTagCache creates a new tag cache
func NewTagCache(ttl time.Duration) *TagCache {
	return &TagCache{
		cache: make(map[string]map[string]string),
		ttl:   ttl,
	}
}

// Get retrieves tags from cache
func (tc *TagCache) Get(key string) (map[string]string, bool) {
	tc.mu.RLock()
	defer tc.mu.RUnlock()
	tags, found := tc.cache[key]
	return tags, found
}

// Set stores tags in cache
func (tc *TagCache) Set(key string, tags map[string]string) {
	tc.mu.Lock()
	defer tc.mu.Unlock()
	tc.cache[key] = tags
}


// MetricCache implements an in-memory cache for metric data
type MetricCache struct {
	cache map[string]*CacheEntry
	mu    sync.RWMutex
	ttl   time.Duration
}

// NewMetricCache creates a new metric cache with specified TTL
func NewMetricCache(ttl time.Duration) *MetricCache {
	return &MetricCache{
		cache: make(map[string]*CacheEntry),
		ttl:   ttl,
	}
}

// Get retrieves a cached entry if it exists and is not expired
func (mc *MetricCache) Get(key string) (*cms.DescribeMetricLastResponse, bool) {
	mc.mu.RLock()
	defer mc.mu.RUnlock()

	entry, exists := mc.cache[key]
	if !exists || entry.IsExpired() {
		return nil, false
	}
	return entry.Data, true
}

// Set stores a cache entry
func (mc *MetricCache) Set(key string, data *cms.DescribeMetricLastResponse) {
	mc.mu.Lock()
	defer mc.mu.Unlock()

	mc.cache[key] = &CacheEntry{
		Data:      data,
		Timestamp: time.Now(),
		TTL:       mc.ttl,
	}
}

// Clear removes expired entries from cache
func (mc *MetricCache) Clear() {
	mc.mu.Lock()
	defer mc.mu.Unlock()

	for key, entry := range mc.cache {
		if entry.IsExpired() {
			delete(mc.cache, key)
		}
	}
}

// Client wraps the Alicloud CMS client with additional functionality
type Client struct {
	cmsClient   *cms.Client
	slbClient   *slb.Client
	slbClients  map[string]*slb.Client // Multi-region SLB clients
	config      *config.AlicloudConfig
	rateLimiter *RateLimiter
	cache       *MetricCache
	tagCache    *TagCache // Add tag cache
	mu          sync.RWMutex
}

// RateLimiter implements a simple rate limiter
type RateLimiter struct {
	tokens   chan struct{}
	ticker   *time.Ticker
	ctx      context.Context
	cancel   context.CancelFunc
	requests int
	burst    int
}

// NewClient creates a new Alicloud client
func NewClient(cfg *config.AlicloudConfig) (*Client, error) {
	cmsClient, err := cms.NewClientWithAccessKey(
		cfg.Region,
		cfg.AccessKeyID,
		cfg.AccessKeySecret,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create CMS client: %w", err)
	}

	slbClient, err := slb.NewClientWithAccessKey(
		cfg.Region,
		cfg.AccessKeyID,
		cfg.AccessKeySecret,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create SLB client: %w", err)
	}

	// Create SLB clients for multiple regions
	regions := cfg.Regions
	if len(regions) == 0 {
		// Fallback to primary region if no regions specified
		regions = []string{cfg.Region}
	}
	slbClients := make(map[string]*slb.Client)
	for _, region := range regions {
		regionClient, err := slb.NewClientWithAccessKey(
			region,
			cfg.AccessKeyID,
			cfg.AccessKeySecret,
		)
		if err != nil {
			// Log warning but continue with other regions
			continue
		}
		slbClients[region] = regionClient
	}

	// Create rate limiter
	rateLimiter := NewRateLimiter(cfg.RateLimit.RequestsPerSecond, cfg.RateLimit.Burst)

	// Create cache with 30 second TTL
	cache := NewMetricCache(30 * time.Second)

	// Create tag cache with 5 minute TTL
	tagCache := NewTagCache(5 * time.Minute)

	return &Client{
		cmsClient:   cmsClient,
		slbClient:   slbClient,
		slbClients:  slbClients,
		cache:       cache,
		tagCache:    tagCache, // Add tag cache
		config:      cfg,
		rateLimiter: rateLimiter,
	}, nil
}

// NewRateLimiter creates a new rate limiter
func NewRateLimiter(requestsPerSecond, burst int) *RateLimiter {
	ctx, cancel := context.WithCancel(context.Background())

	rl := &RateLimiter{
		tokens:   make(chan struct{}, burst),
		ticker:   time.NewTicker(time.Second / time.Duration(requestsPerSecond)),
		ctx:      ctx,
		cancel:   cancel,
		requests: requestsPerSecond,
		burst:    burst,
	}

	// Fill initial tokens
	for i := 0; i < burst; i++ {
		rl.tokens <- struct{}{}
	}

	// Start token refill goroutine
	go rl.refillTokens()

	return rl
}

// refillTokens refills the token bucket
func (rl *RateLimiter) refillTokens() {
	for {
		select {
		case <-rl.ctx.Done():
			return
		case <-rl.ticker.C:
			select {
			case rl.tokens <- struct{}{}:
			default:
				// Token bucket is full, skip
			}
		}
	}
}

// Wait waits for a token to become available with timeout
func (rl *RateLimiter) Wait(ctx context.Context) error {
	// Create a timeout context (5 seconds max wait)
	timeoutCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	
	select {
	case <-timeoutCtx.Done():
		return timeoutCtx.Err()
	case <-rl.tokens:
		return nil
	}
}

// Close closes the rate limiter
func (rl *RateLimiter) Close() {
	rl.cancel()
	rl.ticker.Stop()
}

// GetMetricData retrieves metric data from Alicloud CMS with caching
func (c *Client) GetMetricData(ctx context.Context, namespace, metricName string) (*cms.DescribeMetricLastResponse, error) {
	// Create cache key
	cacheKey := fmt.Sprintf("%s:%s", namespace, metricName)

	// Check cache first
	if cachedData, found := c.cache.Get(cacheKey); found {
		return cachedData, nil
	}

	// Wait for rate limiter
	if err := c.rateLimiter.Wait(ctx); err != nil {
		return nil, fmt.Errorf("rate limiter wait failed: %w", err)
	}

	c.mu.RLock()
	defer c.mu.RUnlock()

	request := cms.CreateDescribeMetricLastRequest()
	request.Scheme = "https"
	request.MetricName = metricName
	request.Namespace = namespace
	request.AcceptFormat = "json"

	response, err := c.cmsClient.DescribeMetricLast(request)
	if err != nil {
		return nil, fmt.Errorf("failed to get metric data for %s/%s: %w", namespace, metricName, err)
	}

	// Cache the response
	c.cache.Set(cacheKey, response)

	return response, nil
}

// GetMetricDataWithDimensions retrieves metric data with specific dimensions
func (c *Client) GetMetricDataWithDimensions(ctx context.Context, namespace, metricName string, dimensions map[string]string) (*cms.DescribeMetricLastResponse, error) {
	// Wait for rate limiter
	if err := c.rateLimiter.Wait(ctx); err != nil {
		return nil, fmt.Errorf("rate limiter wait failed: %w", err)
	}

	c.mu.RLock()
	defer c.mu.RUnlock()

	request := cms.CreateDescribeMetricLastRequest()
	request.Scheme = "https"
	request.MetricName = metricName
	request.Namespace = namespace
	request.AcceptFormat = "json"

	// Add dimensions if provided
	if len(dimensions) > 0 {
		dimensionsJSON := "["
		first := true
		for _, value := range dimensions {
			if !first {
				dimensionsJSON += ","
			}
			dimensionsJSON += fmt.Sprintf(`{"instanceId":"%s"}`, value)
			first = false
		}
		dimensionsJSON += "]"
		request.Dimensions = dimensionsJSON
	}

	response, err := c.cmsClient.DescribeMetricLast(request)
	if err != nil {
		return nil, fmt.Errorf("failed to get metric data for %s/%s: %w", namespace, metricName, err)
	}

	return response, nil
}

// Close closes the client and releases resources
func (c *Client) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.rateLimiter != nil {
		c.rateLimiter.Close()
	}
}

// GetRegion returns the primary region configured for this client
func (c *Client) GetRegion() string {
	return c.config.Region
}

// Health checks the health of the client
func (c *Client) Health(ctx context.Context) error {
	// Try to make a simple request to test connectivity
	request := cms.CreateDescribeMetricMetaListRequest()
	request.Scheme = "https"
	request.Namespace = "acs_ecs_dashboard"
	request.PageSize = "1"

	_, err := c.cmsClient.DescribeMetricMetaList(request)
	if err != nil {
		return fmt.Errorf("health check failed: %w", err)
	}

	return nil
}

// GetSLBInstanceTags retrieves tags for SLB instances using DescribeLoadBalancers API
// This method implements caching and batch processing for optimal performance
func (c *Client) GetSLBInstanceTags(ctx context.Context, instanceIDs []string) (map[string]map[string]string, error) {
	if len(instanceIDs) == 0 {
		return make(map[string]map[string]string), nil
	}

	tagsMap := make(map[string]map[string]string)
	uncachedIDs := make([]string, 0)

	// Check cache first to avoid unnecessary API calls
	for _, id := range instanceIDs {
		if tags, found := c.tagCache.Get(id); found {
			tagsMap[id] = tags
		} else {
			uncachedIDs = append(uncachedIDs, id)
		}
	}

	// If all instances are cached, return immediately
	if len(uncachedIDs) == 0 {
		return tagsMap, nil
	}

	// Create a set of uncached IDs for efficient lookup and removal
	uncachedSet := make(map[string]bool)
	for _, id := range uncachedIDs {
		uncachedSet[id] = true
	}

	// Iterate through all configured regions to find instances
	for region, slbClient := range c.slbClients {
		// Early exit if all instances have been found
		if len(uncachedSet) == 0 {
			break
		}

		// Apply rate limiting before API call
		if err := c.rateLimiter.Wait(ctx); err != nil {
			return tagsMap, fmt.Errorf("rate limiter error: %w", err)
		}

		// Create and configure the DescribeLoadBalancers request
		// Note: DescribeLoadBalancers API doesn't support batch querying by LoadBalancerId
		// We need to query all load balancers and filter by the ones we need
		request := slb.CreateDescribeLoadBalancersRequest()
		request.Scheme = "https"
		// Don't set LoadBalancerId - query all load balancers in the region

		// Execute the API call
		response, err := slbClient.DescribeLoadBalancers(request)
		if err != nil {
			// Log error with region context but continue to next region
			fmt.Printf("Failed to describe load balancers in region %s: %v\n", region, err)
			continue
		}

		// Process each load balancer in the response
		for _, lb := range response.LoadBalancers.LoadBalancer {
			// Only process load balancers that we're looking for
			if _, needed := uncachedSet[lb.LoadBalancerId]; !needed {
				continue
			}

			// Extract tags from the load balancer
			instanceTags := make(map[string]string)
			for _, tag := range lb.Tags.Tag {
				instanceTags[tag.TagKey] = tag.TagValue
			}
			
			// Store in result map and cache
			tagsMap[lb.LoadBalancerId] = instanceTags
			c.tagCache.Set(lb.LoadBalancerId, instanceTags)

			// Remove found instance from uncached set
			delete(uncachedSet, lb.LoadBalancerId)
		}
	}

	// Cache empty tags for instances that weren't found to avoid repeated API calls
	for remainingID := range uncachedSet {
		emptyTags := make(map[string]string)
		tagsMap[remainingID] = emptyTags
		c.tagCache.Set(remainingID, emptyTags)
	}

	return tagsMap, nil
}
