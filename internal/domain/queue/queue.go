package queue

import (
	"context"
	"time"

	"github.com/personal/home-work-ad-process/internal/domain/ad"
)

// QueueItem represents an item in the processing queue
type QueueItem struct {
	adID      ad.AdID
	priority  ad.Priority
	timestamp time.Time
	score     float64
}

// NewQueueItem creates a new queue item
func NewQueueItem(adID ad.AdID, priority ad.Priority, timestamp time.Time) *QueueItem {
	return &QueueItem{
		adID:      adID,
		priority:  priority,
		timestamp: timestamp,
		score:     calculateScore(priority, timestamp),
	}
}

// Getters
func (qi *QueueItem) AdID() ad.AdID       { return qi.adID }
func (qi *QueueItem) Priority() ad.Priority { return qi.priority }
func (qi *QueueItem) Timestamp() time.Time { return qi.timestamp }
func (qi *QueueItem) Score() float64      { return qi.score }

// UpdatePriority updates the priority and recalculates score
func (qi *QueueItem) UpdatePriority(newPriority ad.Priority) {
	qi.priority = newPriority
	qi.score = calculateScore(newPriority, qi.timestamp)
}

// calculateScore generates a score for queue ordering
// Higher priority gets higher score, older items get slight boost for FIFO within priority
func calculateScore(priority ad.Priority, timestamp time.Time) float64 {
	// Base score from priority (multiply by large number to ensure priority dominates)
	baseScore := float64(priority) * 1000000

	// Add timestamp-based micro-adjustment for FIFO within priority
	// Older timestamps get slightly higher scores
	timestampBoost := float64(timestamp.Unix()) / 1000000000 // Very small adjustment
	
	return baseScore + timestampBoost
}

// QueueConfig holds configuration for queue behavior
type QueueConfig struct {
	AntiStarvationEnabled bool
	MaxWaitTime          time.Duration
	WorkerCount          int
	BatchSize            int
	ProcessingTimeout    time.Duration
}

// Manager defines the interface for queue operations
type Manager interface {
	// Enqueue adds an ad to the processing queue
	Enqueue(ctx context.Context, adID ad.AdID, priority ad.Priority) error
	
	// Dequeue removes and returns the next ad to process
	Dequeue(ctx context.Context) (*QueueItem, error)
	
	// DequeueBatch removes and returns multiple ads to process
	DequeueBatch(ctx context.Context, batchSize int) ([]*QueueItem, error)
	
	// UpdatePriority changes the priority of a queued ad
	UpdatePriority(ctx context.Context, adID ad.AdID, newPriority ad.Priority) error
	
	// Remove removes an ad from the queue
	Remove(ctx context.Context, adID ad.AdID) error
	
	// GetPosition returns the position of an ad in the queue
	GetPosition(ctx context.Context, adID ad.AdID) (int, error)
	
	// GetNext returns the next N ads that would be processed
	GetNext(ctx context.Context, count int) ([]*QueueItem, error)
	
	// GetSize returns the current queue size
	GetSize(ctx context.Context) (int64, error)
	
	// GetSizeByPriority returns queue size for each priority level
	GetSizeByPriority(ctx context.Context) (map[ad.Priority]int64, error)
	
	// ApplyAntiStarvation applies anti-starvation logic to boost priorities
	ApplyAntiStarvation(ctx context.Context) error
	
	// UpdateConfig updates queue configuration
	UpdateConfig(ctx context.Context, config QueueConfig) error
	
	// GetConfig returns current queue configuration
	GetConfig(ctx context.Context) (*QueueConfig, error)
}