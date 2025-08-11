package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/personal/home-work-ad-process/internal/domain/ad"
)

// HighPerformanceCache implements multi-level caching for 1M+ RPS
type HighPerformanceCache struct {
	redis  *redis.Client
	l1     *sync.Map // In-memory L1 cache
	l2     *redis.Client // Redis L2 cache
	config *CacheConfig
	stats  *CacheStats
}

type CacheConfig struct {
	L1TTL         time.Duration // L1 cache TTL (default: 1 minute)
	L2TTL         time.Duration // L2 cache TTL (default: 10 minutes)
	L1MaxItems    int           // Maximum items in L1 cache
	EnableL1      bool          // Enable L1 in-memory cache
	EnableL2      bool          // Enable L2 Redis cache
	BatchSize     int           // Batch size for bulk operations
	PrefetchCount int           // Number of items to prefetch
}

type CacheStats struct {
	L1Hits   int64
	L1Misses int64
	L2Hits   int64
	L2Misses int64
	DBHits   int64
	mutex    sync.RWMutex
}

type CacheItem struct {
	Data      interface{}
	ExpiresAt time.Time
}

// NewHighPerformanceCache creates a new high-performance cache
func NewHighPerformanceCache(redisClient *redis.Client, config *CacheConfig) *HighPerformanceCache {
	if config == nil {
		config = &CacheConfig{
			L1TTL:         1 * time.Minute,
			L2TTL:         10 * time.Minute,
			L1MaxItems:    10000,
			EnableL1:      true,
			EnableL2:      true,
			BatchSize:     100,
			PrefetchCount: 50,
		}
	}

	return &HighPerformanceCache{
		redis:  redisClient,
		l1:     &sync.Map{},
		l2:     redisClient,
		config: config,
		stats:  &CacheStats{},
	}
}

// GetAd retrieves an ad using multi-level caching
func (c *HighPerformanceCache) GetAd(ctx context.Context, adID ad.AdID) (*ad.Ad, error) {
	key := fmt.Sprintf("ad:%s", adID.String())
	
	// L1 Cache (in-memory)
	if c.config.EnableL1 {
		if item, ok := c.l1.Load(key); ok {
			cached := item.(*CacheItem)
			if time.Now().Before(cached.ExpiresAt) {
				c.stats.recordL1Hit()
				return cached.Data.(*ad.Ad), nil
			}
			// Expired, remove from L1
			c.l1.Delete(key)
		}
		c.stats.recordL1Miss()
	}

	// L2 Cache (Redis)
	if c.config.EnableL2 {
		data, err := c.redis.Get(ctx, key).Result()
		if err == nil {
			var adEntity ad.Ad
			if json.Unmarshal([]byte(data), &adEntity) == nil {
				c.stats.recordL2Hit()
				
				// Populate L1 cache
				if c.config.EnableL1 {
					c.setL1(key, &adEntity, c.config.L1TTL)
				}
				
				return &adEntity, nil
			}
		}
		c.stats.recordL2Miss()
	}

	c.stats.recordDBHit()
	return nil, fmt.Errorf("ad not found in cache")
}

// SetAd stores an ad in multi-level cache
func (c *HighPerformanceCache) SetAd(ctx context.Context, adEntity *ad.Ad) error {
	key := fmt.Sprintf("ad:%s", adEntity.ID().String())
	
	// Set in L2 (Redis) first
	if c.config.EnableL2 {
		data, err := json.Marshal(adEntity)
		if err != nil {
			return fmt.Errorf("failed to marshal ad: %w", err)
		}
		
		if err := c.redis.Set(ctx, key, data, c.config.L2TTL).Err(); err != nil {
			return fmt.Errorf("failed to set ad in L2 cache: %w", err)
		}
	}

	// Set in L1 (in-memory)
	if c.config.EnableL1 {
		c.setL1(key, adEntity, c.config.L1TTL)
	}

	return nil
}

// SetAdBatch stores multiple ads in batch for better performance
func (c *HighPerformanceCache) SetAdBatch(ctx context.Context, ads []*ad.Ad) error {
	if len(ads) == 0 {
		return nil
	}

	// Batch operations for Redis
	if c.config.EnableL2 {
		pipe := c.redis.Pipeline()
		
		for _, adEntity := range ads {
			key := fmt.Sprintf("ad:%s", adEntity.ID().String())
			data, err := json.Marshal(adEntity)
			if err != nil {
				continue // Skip invalid ads
			}
			pipe.Set(ctx, key, data, c.config.L2TTL)
		}
		
		if _, err := pipe.Exec(ctx); err != nil {
			return fmt.Errorf("failed to execute batch set: %w", err)
		}
	}

	// Set in L1 cache
	if c.config.EnableL1 {
		for _, adEntity := range ads {
			key := fmt.Sprintf("ad:%s", adEntity.ID().String())
			c.setL1(key, adEntity, c.config.L1TTL)
		}
	}

	return nil
}

// GetAdBatch retrieves multiple ads efficiently
func (c *HighPerformanceCache) GetAdBatch(ctx context.Context, adIDs []ad.AdID) ([]*ad.Ad, []ad.AdID, error) {
	var foundAds []*ad.Ad
	var missingIDs []ad.AdID
	var l1Misses []ad.AdID

	// Try L1 cache first
	if c.config.EnableL1 {
		for _, adID := range adIDs {
			key := fmt.Sprintf("ad:%s", adID.String())
			if item, ok := c.l1.Load(key); ok {
				cached := item.(*CacheItem)
				if time.Now().Before(cached.ExpiresAt) {
					foundAds = append(foundAds, cached.Data.(*ad.Ad))
					c.stats.recordL1Hit()
					continue
				}
				c.l1.Delete(key) // Remove expired item
			}
			l1Misses = append(l1Misses, adID)
			c.stats.recordL1Miss()
		}
	} else {
		l1Misses = adIDs
	}

	// Try L2 cache for L1 misses
	if c.config.EnableL2 && len(l1Misses) > 0 {
		keys := make([]string, len(l1Misses))
		for i, adID := range l1Misses {
			keys[i] = fmt.Sprintf("ad:%s", adID.String())
		}

		results, err := c.redis.MGet(ctx, keys...).Result()
		if err != nil {
			return foundAds, l1Misses, fmt.Errorf("failed to get batch from L2: %w", err)
		}

		for i, result := range results {
			if result != nil {
				var adEntity ad.Ad
				if data, ok := result.(string); ok {
					if json.Unmarshal([]byte(data), &adEntity) == nil {
						foundAds = append(foundAds, &adEntity)
						c.stats.recordL2Hit()

						// Populate L1 cache
						if c.config.EnableL1 {
							key := keys[i]
							c.setL1(key, &adEntity, c.config.L1TTL)
						}
						continue
					}
				}
			}
			missingIDs = append(missingIDs, l1Misses[i])
			c.stats.recordL2Miss()
		}
	} else {
		missingIDs = l1Misses
	}

	return foundAds, missingIDs, nil
}

// DeleteAd removes an ad from all cache levels
func (c *HighPerformanceCache) DeleteAd(ctx context.Context, adID ad.AdID) error {
	key := fmt.Sprintf("ad:%s", adID.String())
	
	// Delete from L1
	if c.config.EnableL1 {
		c.l1.Delete(key)
	}

	// Delete from L2
	if c.config.EnableL2 {
		if err := c.redis.Del(ctx, key).Err(); err != nil {
			return fmt.Errorf("failed to delete from L2 cache: %w", err)
		}
	}

	return nil
}

// InvalidatePattern removes all keys matching a pattern
func (c *HighPerformanceCache) InvalidatePattern(ctx context.Context, pattern string) error {
	if !c.config.EnableL2 {
		return nil
	}

	// Get all keys matching pattern
	keys, err := c.redis.Keys(ctx, pattern).Result()
	if err != nil {
		return fmt.Errorf("failed to get keys for pattern: %w", err)
	}

	if len(keys) == 0 {
		return nil
	}

	// Delete keys in batches
	for i := 0; i < len(keys); i += c.config.BatchSize {
		end := i + c.config.BatchSize
		if end > len(keys) {
			end = len(keys)
		}

		batch := keys[i:end]
		if err := c.redis.Del(ctx, batch...).Err(); err != nil {
			return fmt.Errorf("failed to delete batch: %w", err)
		}

		// Also remove from L1
		if c.config.EnableL1 {
			for _, key := range batch {
				c.l1.Delete(key)
			}
		}
	}

	return nil
}

// PrefetchAds preloads ads into cache for better performance
func (c *HighPerformanceCache) PrefetchAds(ctx context.Context, adIDs []ad.AdID, loader func([]ad.AdID) ([]*ad.Ad, error)) error {
	if len(adIDs) == 0 {
		return nil
	}

	// Check which ads are not in cache
	_, missingIDs, err := c.GetAdBatch(ctx, adIDs)
	if err != nil {
		return fmt.Errorf("failed to check cache: %w", err)
	}

	if len(missingIDs) == 0 {
		return nil // All ads already cached
	}

	// Load missing ads
	ads, err := loader(missingIDs)
	if err != nil {
		return fmt.Errorf("failed to load ads: %w", err)
	}

	// Store in cache
	return c.SetAdBatch(ctx, ads)
}

// WarmupCache preloads frequently accessed ads
func (c *HighPerformanceCache) WarmupCache(ctx context.Context, loader func(int) ([]*ad.Ad, error)) error {
	ads, err := loader(c.config.PrefetchCount)
	if err != nil {
		return fmt.Errorf("failed to load ads for warmup: %w", err)
	}

	return c.SetAdBatch(ctx, ads)
}

// GetStats returns cache performance statistics
func (c *HighPerformanceCache) GetStats() CacheStats {
	c.stats.mutex.RLock()
	defer c.stats.mutex.RUnlock()
	
	return CacheStats{
		L1Hits:   c.stats.L1Hits,
		L1Misses: c.stats.L1Misses,
		L2Hits:   c.stats.L2Hits,
		L2Misses: c.stats.L2Misses,
		DBHits:   c.stats.DBHits,
	}
}

// ResetStats resets all cache statistics
func (c *HighPerformanceCache) ResetStats() {
	c.stats.mutex.Lock()
	defer c.stats.mutex.Unlock()
	
	c.stats.L1Hits = 0
	c.stats.L1Misses = 0
	c.stats.L2Hits = 0
	c.stats.L2Misses = 0
	c.stats.DBHits = 0
}

// CleanupExpired removes expired items from L1 cache
func (c *HighPerformanceCache) CleanupExpired() {
	if !c.config.EnableL1 {
		return
	}

	now := time.Now()
	var toDelete []interface{}

	c.l1.Range(func(key, value interface{}) bool {
		item := value.(*CacheItem)
		if now.After(item.ExpiresAt) {
			toDelete = append(toDelete, key)
		}
		return true
	})

	for _, key := range toDelete {
		c.l1.Delete(key)
	}
}

// Helper methods
func (c *HighPerformanceCache) setL1(key string, data interface{}, ttl time.Duration) {
	// Check L1 cache size limit
	size := 0
	c.l1.Range(func(_, _ interface{}) bool {
		size++
		return size < c.config.L1MaxItems
	})

	if size >= c.config.L1MaxItems {
		// Simple eviction: remove oldest items
		// In production, use LRU or LFU eviction policy
		c.l1.Range(func(k, v interface{}) bool {
			c.l1.Delete(k)
			size--
			return size >= c.config.L1MaxItems*9/10 // Keep 90% of max
		})
	}

	item := &CacheItem{
		Data:      data,
		ExpiresAt: time.Now().Add(ttl),
	}
	c.l1.Store(key, item)
}

func (s *CacheStats) recordL1Hit() {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.L1Hits++
}

func (s *CacheStats) recordL1Miss() {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.L1Misses++
}

func (s *CacheStats) recordL2Hit() {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.L2Hits++
}

func (s *CacheStats) recordL2Miss() {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.L2Misses++
}

func (s *CacheStats) recordDBHit() {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.DBHits++
}

// CalculateHitRatio calculates overall cache hit ratio
func (s *CacheStats) CalculateHitRatio() float64 {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	
	totalHits := s.L1Hits + s.L2Hits
	totalRequests := totalHits + s.DBHits
	
	if totalRequests == 0 {
		return 0
	}
	
	return float64(totalHits) / float64(totalRequests)
}