package service

import (
	"context"
	"fmt"
	"time"

	"github.com/personal/home-work-ad-process/internal/domain/ad"
	"github.com/personal/home-work-ad-process/internal/domain/queue"
)

// AdService provides business logic for ad operations
type AdService struct {
	adRepo      ad.Repository
	queueManager queue.Manager
}

// NewAdService creates a new AdService
func NewAdService(adRepo ad.Repository, queueManager queue.Manager) *AdService {
	return &AdService{
		adRepo:      adRepo,
		queueManager: queueManager,
	}
}

// CreateAdRequest represents a request to create a new ad
type CreateAdRequest struct {
	Title          string   `json:"title" validate:"required"`
	GameFamily     string   `json:"gameFamily" validate:"required"`
	TargetAudience []string `json:"targetAudience" validate:"required,min=1"`
	Priority       *int     `json:"priority,omitempty" validate:"omitempty,min=1,max=5"`
	MaxWaitTime    *int     `json:"maxWaitTime,omitempty" validate:"omitempty,min=1"`
}

// CreateAdResponse represents the response after creating an ad
type CreateAdResponse struct {
	AdID                 string    `json:"adId"`
	Status               string    `json:"status"`
	Priority             int       `json:"priority"`
	Position             int       `json:"position"`
	EstimatedProcessTime time.Time `json:"estimatedProcessTime"`
}

// CreateAd creates a new ad and adds it to the queue
func (s *AdService) CreateAd(ctx context.Context, req *CreateAdRequest) (*CreateAdResponse, error) {
	// Set defaults
	priority := ad.PriorityNormal
	if req.Priority != nil {
		priority = ad.Priority(*req.Priority)
	}
	
	maxWaitTime := 5 * time.Minute
	if req.MaxWaitTime != nil {
		maxWaitTime = time.Duration(*req.MaxWaitTime) * time.Second
	}
	
	// Create ad entity
	newAd, err := ad.NewAd(req.Title, req.GameFamily, req.TargetAudience, priority, maxWaitTime)
	if err != nil {
		return nil, fmt.Errorf("failed to create ad: %w", err)
	}
	
	// Save to repository
	if err := s.adRepo.Save(ctx, newAd); err != nil {
		return nil, fmt.Errorf("failed to save ad: %w", err)
	}
	
	// Add to queue
	if err := s.queueManager.Enqueue(ctx, newAd.ID(), newAd.Priority()); err != nil {
		// TODO: Consider compensation - remove from repo if queue fails
		return nil, fmt.Errorf("failed to enqueue ad: %w", err)
	}
	
	// Get queue position
	position, err := s.queueManager.GetPosition(ctx, newAd.ID())
	if err != nil {
		// Non-fatal error, set default position
		position = -1
	}
	
	// Estimate processing time (rough calculation)
	estimatedTime := time.Now().Add(time.Duration(position*3) * time.Second)
	
	return &CreateAdResponse{
		AdID:                 newAd.ID().String(),
		Status:               string(newAd.Status()),
		Priority:             int(newAd.Priority()),
		Position:             position,
		EstimatedProcessTime: estimatedTime,
	}, nil
}

// GetAdStatusResponse represents the response for ad status query
type GetAdStatusResponse struct {
	AdID                string     `json:"adId"`
	Title               string     `json:"title"`
	GameFamily          string     `json:"gameFamily"`
	Status              string     `json:"status"`
	Priority            int        `json:"priority"`
	Position            *int       `json:"position,omitempty"`
	WaitTime            string     `json:"waitTime"`
	CreatedAt           time.Time  `json:"createdAt"`
	ProcessedAt         *time.Time `json:"processedAt,omitempty"`
}

// GetAdStatus retrieves the current status of an ad
func (s *AdService) GetAdStatus(ctx context.Context, adID string) (*GetAdStatusResponse, error) {
	parsedID, err := ad.ParseAdID(adID)
	if err != nil {
		return nil, fmt.Errorf("invalid ad ID: %w", err)
	}
	
	adEntity, err := s.adRepo.FindByID(ctx, parsedID)
	if err != nil {
		return nil, fmt.Errorf("failed to find ad: %w", err)
	}
	
	response := &GetAdStatusResponse{
		AdID:        adEntity.ID().String(),
		Title:       adEntity.Title(),
		GameFamily:  adEntity.GameFamily(),
		Status:      string(adEntity.Status()),
		Priority:    int(adEntity.Priority()),
		WaitTime:    adEntity.WaitTime().String(),
		CreatedAt:   adEntity.CreatedAt(),
		ProcessedAt: adEntity.ProcessedAt(),
	}
	
	// Get queue position if still queued
	if adEntity.Status() == ad.AdStatusQueued {
		position, err := s.queueManager.GetPosition(ctx, parsedID)
		if err == nil {
			response.Position = &position
		}
	}
	
	return response, nil
}

// ChangePriorityForGameFamily changes priority for all ads in a game family
func (s *AdService) ChangePriorityForGameFamily(ctx context.Context, gameFamily string, newPriority ad.Priority) (int, error) {
	if !newPriority.IsValid() {
		return 0, fmt.Errorf("invalid priority: %d", newPriority)
	}
	
	// Find all ads in the game family
	ads, err := s.adRepo.FindByGameFamily(ctx, gameFamily)
	if err != nil {
		return 0, fmt.Errorf("failed to find ads by game family: %w", err)
	}
	
	var updatedCount int
	var adIDs []ad.AdID
	
	// Filter ads that can have priority changed
	for _, adEntity := range ads {
		if adEntity.Status() == ad.AdStatusQueued {
			adIDs = append(adIDs, adEntity.ID())
		}
	}
	
	if len(adIDs) == 0 {
		return 0, nil
	}
	
	// Update priority in batch
	if err := s.adRepo.UpdatePriorityBatch(ctx, adIDs, newPriority); err != nil {
		return 0, fmt.Errorf("failed to update priorities: %w", err)
	}
	
	// Update queue priorities
	for _, adID := range adIDs {
		if err := s.queueManager.UpdatePriority(ctx, adID, newPriority); err != nil {
			// Log error but don't fail the whole operation
			// TODO: Add proper logging
			continue
		}
		updatedCount++
	}
	
	return updatedCount, nil
}

// ChangePriorityForOlderAds changes priority for ads older than specified duration
func (s *AdService) ChangePriorityForOlderAds(ctx context.Context, olderThan time.Duration, newPriority ad.Priority) (int, error) {
	if !newPriority.IsValid() {
		return 0, fmt.Errorf("invalid priority: %d", newPriority)
	}
	
	threshold := time.Now().Add(-olderThan)
	
	// Find old ads
	ads, err := s.adRepo.FindOlderThan(ctx, threshold)
	if err != nil {
		return 0, fmt.Errorf("failed to find old ads: %w", err)
	}
	
	var updatedCount int
	var adIDs []ad.AdID
	
	// Filter ads that can have priority changed
	for _, adEntity := range ads {
		if adEntity.Status() == ad.AdStatusQueued {
			adIDs = append(adIDs, adEntity.ID())
		}
	}
	
	if len(adIDs) == 0 {
		return 0, nil
	}
	
	// Update priority in batch
	if err := s.adRepo.UpdatePriorityBatch(ctx, adIDs, newPriority); err != nil {
		return 0, fmt.Errorf("failed to update priorities: %w", err)
	}
	
	// Update queue priorities
	for _, adID := range adIDs {
		if err := s.queueManager.UpdatePriority(ctx, adID, newPriority); err != nil {
			// Log error but don't fail the whole operation
			continue
		}
		updatedCount++
	}
	
	return updatedCount, nil
}

// GetQueueDistribution returns the distribution of ads by priority
func (s *AdService) GetQueueDistribution(ctx context.Context) (map[ad.Priority]int64, error) {
	return s.queueManager.GetSizeByPriority(ctx)
}

// GetNextAds returns the next N ads to be processed
func (s *AdService) GetNextAds(ctx context.Context, count int) ([]*queue.QueueItem, error) {
	return s.queueManager.GetNext(ctx, count)
}

// GetWaitingAds returns ads waiting longer than specified duration
func (s *AdService) GetWaitingAds(ctx context.Context, waitTime time.Duration) ([]*ad.Ad, error) {
	threshold := time.Now().Add(-waitTime)
	return s.adRepo.FindOlderThan(ctx, threshold)
}