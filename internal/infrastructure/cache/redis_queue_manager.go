package cache

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/personal/home-work-ad-process/internal/domain/ad"
	"github.com/personal/home-work-ad-process/internal/domain/queue"
)

// RedisQueueManager implements the queue.Manager interface using Redis
type RedisQueueManager struct {
	client     *redis.Client
	config     *queue.QueueConfig
	shardCount int
}

// NewRedisQueueManager creates a new RedisQueueManager
func NewRedisQueueManager(client *redis.Client, config *queue.QueueConfig, shardCount int) *RedisQueueManager {
	return &RedisQueueManager{
		client:     client,
		config:     config,
		shardCount: shardCount,
	}
}

// getShardKey returns the shard key for an ad ID
func (r *RedisQueueManager) getShardKey(adID ad.AdID) string {
	// Simple hash-based sharding
	hash := 0
	for _, c := range adID.String() {
		hash = int(c) + ((hash << 5) - hash)
	}
	
	shard := hash % r.shardCount
	if shard < 0 {
		shard = -shard
	}
	
	return fmt.Sprintf("queue:shard:%d", shard)
}

// calculateScore calculates the score for Redis sorted set
// Higher priority gets higher scores, older timestamp within same priority gets higher scores
func (r *RedisQueueManager) calculateScore(priority ad.Priority, timestamp time.Time) float64 {
	// Base score from priority (multiply by large number to ensure priority dominates)
	baseScore := float64(priority) * 10000000000
	
	// For FIFO within priority: use max timestamp minus current timestamp
	// This ensures older items get higher scores within the same priority
	maxTime := float64(9999999999) // Year 2286 approximately
	timestampScore := maxTime - float64(timestamp.Unix())
	
	return baseScore + timestampScore
}

// Enqueue adds an ad to the processing queue
func (r *RedisQueueManager) Enqueue(ctx context.Context, adID ad.AdID, priority ad.Priority) error {
	shardKey := r.getShardKey(adID)
	score := r.calculateScore(priority, time.Now())
	
	// Add to sorted set with priority+timestamp score
	err := r.client.ZAdd(ctx, shardKey, redis.Z{
		Score:  score,
		Member: adID.String(),
	}).Err()
	
	if err != nil {
		return fmt.Errorf("failed to enqueue ad: %w", err)
	}
	
	// Publish notification for queue processors
	r.client.Publish(ctx, "queue:notifications", fmt.Sprintf("enqueued:%s", adID.String()))
	
	return nil
}

// Dequeue removes and returns the next ad to process
func (r *RedisQueueManager) Dequeue(ctx context.Context) (*queue.QueueItem, error) {
	// Get all items from all shards and find the highest priority one
	var bestItem *redis.Z
	var bestShardKey string
	
	for i := 0; i < r.shardCount; i++ {
		shardKey := fmt.Sprintf("queue:shard:%d", i)
		
		// Get highest score item from this shard (without removing)
		result, err := r.client.ZRevRangeWithScores(ctx, shardKey, 0, 0).Result()
		if err != nil {
			if err == redis.Nil {
				continue
			}
			return nil, fmt.Errorf("failed to peek shard %s: %w", shardKey, err)
		}
		
		if len(result) == 0 {
			continue
		}
		
		// Compare with current best
		if bestItem == nil || result[0].Score > bestItem.Score {
			bestItem = &result[0]
			bestShardKey = shardKey
		}
	}
	
	if bestItem == nil {
		return nil, nil // No items in any shard
	}
	
	// Remove the best item from its shard
	err := r.client.ZRem(ctx, bestShardKey, bestItem.Member).Err()
	if err != nil {
		return nil, fmt.Errorf("failed to remove item from %s: %w", bestShardKey, err)
	}
	
	adID, err := ad.ParseAdID(bestItem.Member.(string))
	if err != nil {
		return nil, fmt.Errorf("invalid ad ID in queue: %w", err)
	}
	
	// Extract priority from score
	priority := ad.Priority(int(bestItem.Score / 10000000000))
	
	return queue.NewQueueItem(adID, priority, time.Now()), nil
}

// DequeueBatch removes and returns multiple ads to process
func (r *RedisQueueManager) DequeueBatch(ctx context.Context, batchSize int) ([]*queue.QueueItem, error) {
	// Collect all items from all shards
	var allItems []redis.Z
	var shardMapping = make(map[string]string) // member -> shard key
	
	for i := 0; i < r.shardCount; i++ {
		shardKey := fmt.Sprintf("queue:shard:%d", i)
		
		// Get top items from this shard
		result, err := r.client.ZRevRangeWithScores(ctx, shardKey, 0, int64(batchSize-1)).Result()
		if err != nil && err != redis.Nil {
			return nil, fmt.Errorf("failed to get items from shard %s: %w", shardKey, err)
		}
		
		for _, item := range result {
			allItems = append(allItems, item)
			shardMapping[item.Member.(string)] = shardKey
		}
	}
	
	// Sort all items by score (descending - highest first)
	for i := 0; i < len(allItems); i++ {
		for j := i + 1; j < len(allItems); j++ {
			if allItems[i].Score < allItems[j].Score {
				allItems[i], allItems[j] = allItems[j], allItems[i]
			}
		}
	}
	
	// Take the top batchSize items
	var items []*queue.QueueItem
	var toRemove = make(map[string][]interface{}) // shard -> members to remove
	
	for i := 0; i < batchSize && i < len(allItems); i++ {
		member := allItems[i]
		
		adID, err := ad.ParseAdID(member.Member.(string))
		if err != nil {
			continue
		}
		
		priority := ad.Priority(int(member.Score / 10000000000))
		items = append(items, queue.NewQueueItem(adID, priority, time.Now()))
		
		// Track which items to remove from which shard
		shardKey := shardMapping[member.Member.(string)]
		if toRemove[shardKey] == nil {
			toRemove[shardKey] = make([]interface{}, 0)
		}
		toRemove[shardKey] = append(toRemove[shardKey], member.Member)
	}
	
	// Remove selected items from their respective shards
	for shardKey, members := range toRemove {
		if len(members) > 0 {
			r.client.ZRem(ctx, shardKey, members...)
		}
	}
	
	return items, nil
}

// UpdatePriority changes the priority of a queued ad
func (r *RedisQueueManager) UpdatePriority(ctx context.Context, adID ad.AdID, newPriority ad.Priority) error {
	shardKey := r.getShardKey(adID)
	
	// Check if item exists and get current timestamp
	currentScore, err := r.client.ZScore(ctx, shardKey, adID.String()).Result()
	if err != nil {
		if err == redis.Nil {
			return fmt.Errorf("ad not found in queue: %s", adID.String())
		}
		return fmt.Errorf("failed to get current score: %w", err)
	}
	
	// Extract timestamp from current score
	timestampPart := currentScore - (float64(int(currentScore/10000000000)) * 10000000000)
	
	// Calculate new score with same timestamp but new priority
	newScore := float64(newPriority)*10000000000 + timestampPart
	
	// Update the score
	err = r.client.ZAdd(ctx, shardKey, redis.Z{
		Score:  newScore,
		Member: adID.String(),
	}).Err()
	
	if err != nil {
		return fmt.Errorf("failed to update priority: %w", err)
	}
	
	return nil
}

// Remove removes an ad from the queue
func (r *RedisQueueManager) Remove(ctx context.Context, adID ad.AdID) error {
	shardKey := r.getShardKey(adID)
	
	removed, err := r.client.ZRem(ctx, shardKey, adID.String()).Result()
	if err != nil {
		return fmt.Errorf("failed to remove ad from queue: %w", err)
	}
	
	if removed == 0 {
		return fmt.Errorf("ad not found in queue: %s", adID.String())
	}
	
	return nil
}

// GetPosition returns the position of an ad in the queue
func (r *RedisQueueManager) GetPosition(ctx context.Context, adID ad.AdID) (int, error) {
	shardKey := r.getShardKey(adID)
	
	// Get the score of the target ad
	targetScore, err := r.client.ZScore(ctx, shardKey, adID.String()).Result()
	if err != nil {
		if err == redis.Nil {
			return -1, fmt.Errorf("ad not found in queue: %s", adID.String())
		}
		return -1, fmt.Errorf("failed to get ad score: %w", err)
	}
	
	// Count all items with higher scores across all shards
	position := 0
	for i := 0; i < r.shardCount; i++ {
		shardKey := fmt.Sprintf("queue:shard:%d", i)
		
		// Count items with score > targetScore in this shard
		count, err := r.client.ZCount(ctx, shardKey, fmt.Sprintf("(%.0f", targetScore), "+inf").Result()
		if err == nil {
			position += int(count)
		}
	}
	
	return position + 1, nil // 1-indexed position
}

// hashAdID returns a consistent hash for an ad ID
func (r *RedisQueueManager) hashAdID(adID ad.AdID) int {
	hash := 0
	for _, c := range adID.String() {
		hash = int(c) + ((hash << 5) - hash)
	}
	if hash < 0 {
		hash = -hash
	}
	return hash
}

// GetNext returns the next N ads that would be processed
func (r *RedisQueueManager) GetNext(ctx context.Context, count int) ([]*queue.QueueItem, error) {
	var allItems []redis.Z
	
	// Collect items from all shards
	for i := 0; i < r.shardCount; i++ {
		shardKey := fmt.Sprintf("queue:shard:%d", i)
		
		// Get top items from this shard (without removing)
		items, err := r.client.ZRevRangeWithScores(ctx, shardKey, 0, int64(count-1)).Result()
		if err != nil && err != redis.Nil {
			return nil, fmt.Errorf("failed to get items from shard %s: %w", shardKey, err)
		}
		
		allItems = append(allItems, items...)
	}
	
	// Sort all items by score (descending)
	// Note: In production, consider using a priority queue for better performance
	for i := 0; i < len(allItems); i++ {
		for j := i + 1; j < len(allItems); j++ {
			if allItems[i].Score < allItems[j].Score {
				allItems[i], allItems[j] = allItems[j], allItems[i]
			}
		}
	}
	
	// Convert to QueueItems
	var queueItems []*queue.QueueItem
	for i := 0; i < count && i < len(allItems); i++ {
		adID, err := ad.ParseAdID(allItems[i].Member.(string))
		if err != nil {
			continue
		}
		
		priority := ad.Priority(int(allItems[i].Score / 10000000000))
		queueItems = append(queueItems, queue.NewQueueItem(adID, priority, time.Now()))
	}
	
	return queueItems, nil
}

// GetSize returns the current queue size
func (r *RedisQueueManager) GetSize(ctx context.Context) (int64, error) {
	var totalSize int64
	
	for i := 0; i < r.shardCount; i++ {
		shardKey := fmt.Sprintf("queue:shard:%d", i)
		size, err := r.client.ZCard(ctx, shardKey).Result()
		if err != nil && err != redis.Nil {
			return 0, fmt.Errorf("failed to get size for shard %s: %w", shardKey, err)
		}
		totalSize += size
	}
	
	return totalSize, nil
}

// GetSizeByPriority returns queue size for each priority level
func (r *RedisQueueManager) GetSizeByPriority(ctx context.Context) (map[ad.Priority]int64, error) {
	sizes := make(map[ad.Priority]int64)
	
	for priority := ad.Priority(1); priority <= 5; priority++ {
		minScore := float64(priority) * 10000000000
		maxScore := float64(priority+1)*10000000000 - 1
		
		var totalCount int64
		for i := 0; i < r.shardCount; i++ {
			shardKey := fmt.Sprintf("queue:shard:%d", i)
			count, err := r.client.ZCount(ctx, shardKey, fmt.Sprintf("%.0f", minScore), fmt.Sprintf("%.0f", maxScore)).Result()
			if err != nil && err != redis.Nil {
				continue
			}
			totalCount += count
		}
		
		sizes[priority] = totalCount
	}
	
	return sizes, nil
}

// ApplyAntiStarvation applies anti-starvation logic to boost priorities
func (r *RedisQueueManager) ApplyAntiStarvation(ctx context.Context) error {
	if !r.config.AntiStarvationEnabled {
		return nil
	}
	
	// This is a simplified implementation
	// In production, you'd want to track timestamps more precisely
	
	for i := 0; i < r.shardCount; i++ {
		shardKey := fmt.Sprintf("queue:shard:%d", i)
		
		// Get all items in this shard
		items, err := r.client.ZRangeWithScores(ctx, shardKey, 0, -1).Result()
		if err != nil {
			continue
		}
		
		for _, item := range items {
			// Extract timestamp and priority from score
			score := item.Score
			priority := ad.Priority(int(score / 10000000000))
			timestampPart := score - (float64(priority) * 10000000000)
			
			// Calculate age (timestamp part is (maxTime - actualTimestamp))
			maxTime := float64(9999999999)
			actualTimestamp := maxTime - timestampPart
			estimatedTimestamp := time.Unix(int64(actualTimestamp), 0)
			age := time.Since(estimatedTimestamp)
			
			// Apply starvation boost if needed
			if age > r.config.MaxWaitTime && priority < ad.PriorityHigh {
				boost := int(age.Minutes() / 5) // +1 priority every 5 minutes
				newPriority := priority + ad.Priority(boost)
				if newPriority > ad.PriorityHigh {
					newPriority = ad.PriorityHigh
				}
				
				// Update with boosted priority
				newScore := float64(newPriority)*10000000000 + timestampPart
				r.client.ZAdd(ctx, shardKey, redis.Z{
					Score:  newScore,
					Member: item.Member,
				})
			}
		}
	}
	
	return nil
}

// UpdateConfig updates queue configuration
func (r *RedisQueueManager) UpdateConfig(ctx context.Context, config queue.QueueConfig) error {
	r.config = &config
	
	// Store config in Redis for persistence
	configKey := "queue:config"
	configData := fmt.Sprintf("%t:%d:%d:%d:%d",
		config.AntiStarvationEnabled,
		int(config.MaxWaitTime.Seconds()),
		config.WorkerCount,
		config.BatchSize,
		int(config.ProcessingTimeout.Seconds()))
	
	return r.client.Set(ctx, configKey, configData, 0).Err()
}

// GetConfig returns current queue configuration
func (r *RedisQueueManager) GetConfig(ctx context.Context) (*queue.QueueConfig, error) {
	configKey := "queue:config"
	configData, err := r.client.Get(ctx, configKey).Result()
	if err != nil {
		if err == redis.Nil {
			return r.config, nil // Return current config if not stored
		}
		return nil, fmt.Errorf("failed to get config: %w", err)
	}
	
	// Parse stored config
	parts := strings.Split(configData, ":")
	if len(parts) != 5 {
		return r.config, nil // Return current config if format is invalid
	}
	
	antiStarvation, _ := strconv.ParseBool(parts[0])
	maxWaitTime, _ := strconv.Atoi(parts[1])
	workerCount, _ := strconv.Atoi(parts[2])
	batchSize, _ := strconv.Atoi(parts[3])
	processingTimeout, _ := strconv.Atoi(parts[4])
	
	return &queue.QueueConfig{
		AntiStarvationEnabled: antiStarvation,
		MaxWaitTime:          time.Duration(maxWaitTime) * time.Second,
		WorkerCount:          workerCount,
		BatchSize:            batchSize,
		ProcessingTimeout:    time.Duration(processingTimeout) * time.Second,
	}, nil
}