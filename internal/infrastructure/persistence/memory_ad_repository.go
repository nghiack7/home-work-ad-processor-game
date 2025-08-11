package persistence

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/personal/home-work-ad-process/internal/domain/ad"
)

// MemoryAdRepository is an in-memory implementation for testing
type MemoryAdRepository struct {
	ads map[string]*ad.Ad
	mu  sync.RWMutex
}

// NewMemoryAdRepository creates a new in-memory ad repository
func NewMemoryAdRepository() *MemoryAdRepository {
	return &MemoryAdRepository{
		ads: make(map[string]*ad.Ad),
	}
}

// Save stores an ad in memory
func (r *MemoryAdRepository) Save(ctx context.Context, adEntity *ad.Ad) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.ads[adEntity.ID().String()] = adEntity
	return nil
}

// FindByID retrieves an ad by ID
func (r *MemoryAdRepository) FindByID(ctx context.Context, id ad.AdID) (*ad.Ad, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	
	adEntity, exists := r.ads[id.String()]
	if !exists {
		return nil, fmt.Errorf("ad not found")
	}
	return adEntity, nil
}

// FindByGameFamily retrieves ads by game family
func (r *MemoryAdRepository) FindByGameFamily(ctx context.Context, gameFamily string) ([]*ad.Ad, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	
	var result []*ad.Ad
	for _, adEntity := range r.ads {
		if adEntity.GameFamily() == gameFamily {
			result = append(result, adEntity)
		}
	}
	return result, nil
}

// FindAll retrieves all ads
func (r *MemoryAdRepository) FindAll(ctx context.Context) ([]*ad.Ad, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	
	var result []*ad.Ad
	for _, adEntity := range r.ads {
		result = append(result, adEntity)
	}
	return result, nil
}

// Delete removes an ad from memory
func (r *MemoryAdRepository) Delete(ctx context.Context, id ad.AdID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.ads, id.String())
	return nil
}

// FindByStatus retrieves ads by status
func (r *MemoryAdRepository) FindByStatus(ctx context.Context, status ad.AdStatus) ([]*ad.Ad, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	
	var result []*ad.Ad
	for _, adEntity := range r.ads {
		if adEntity.Status() == status {
			result = append(result, adEntity)
		}
	}
	return result, nil
}

// FindOlderThan retrieves ads created before the specified time
func (r *MemoryAdRepository) FindOlderThan(ctx context.Context, threshold time.Time) ([]*ad.Ad, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	
	var result []*ad.Ad
	for _, adEntity := range r.ads {
		if adEntity.CreatedAt().Before(threshold) {
			result = append(result, adEntity)
		}
	}
	return result, nil
}

// UpdatePriorityBatch updates priority for multiple ads
func (r *MemoryAdRepository) UpdatePriorityBatch(ctx context.Context, ids []ad.AdID, newPriority ad.Priority) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	
	for _, id := range ids {
		if adEntity, exists := r.ads[id.String()]; exists {
			err := adEntity.ChangePriority(newPriority)
			if err != nil {
				return fmt.Errorf("failed to update priority for ad %s: %w", id.String(), err)
			}
		}
	}
	return nil
}

// UpdateStatus updates the status of an ad
func (r *MemoryAdRepository) UpdateStatus(ctx context.Context, id ad.AdID, status ad.AdStatus, version int) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	
	adEntity, exists := r.ads[id.String()]
	if !exists {
		return fmt.Errorf("ad not found")
	}
	
	// For simplicity, we'll skip version checking in memory implementation
	switch status {
	case ad.AdStatusProcessing:
		return adEntity.StartProcessing()
	case ad.AdStatusCompleted:
		return adEntity.CompleteProcessing()
	case ad.AdStatusFailed:
		return adEntity.FailProcessing()
	}
	
	return nil
}

// Count returns the number of ads in memory
func (r *MemoryAdRepository) Count(ctx context.Context) (int64, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return int64(len(r.ads)), nil
}

// CountByStatus returns the number of ads with a specific status
func (r *MemoryAdRepository) CountByStatus(ctx context.Context, status ad.AdStatus) (int64, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	
	count := int64(0)
	for _, adEntity := range r.ads {
		if adEntity.Status() == status {
			count++
		}
	}
	return count, nil
}

// CountByPriority returns the number of ads with a specific priority
func (r *MemoryAdRepository) CountByPriority(ctx context.Context, priority ad.Priority) (int64, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	
	count := int64(0)
	for _, adEntity := range r.ads {
		if adEntity.Priority() == priority {
			count++
		}
	}
	return count, nil
}