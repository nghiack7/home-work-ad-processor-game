package ad

import (
	"context"
	"time"
)

// Repository defines the interface for ad persistence
type Repository interface {
	// Save saves an ad to the repository
	Save(ctx context.Context, ad *Ad) error
	
	// FindByID finds an ad by its ID
	FindByID(ctx context.Context, id AdID) (*Ad, error)
	
	// FindByGameFamily finds all ads for a specific game family
	FindByGameFamily(ctx context.Context, gameFamily string) ([]*Ad, error)
	
	// FindByStatus finds all ads with a specific status
	FindByStatus(ctx context.Context, status AdStatus) ([]*Ad, error)
	
	// FindOlderThan finds all ads created before the specified time
	FindOlderThan(ctx context.Context, threshold time.Time) ([]*Ad, error)
	
	// UpdatePriorityBatch updates priority for multiple ads atomically
	UpdatePriorityBatch(ctx context.Context, ids []AdID, newPriority Priority) error
	
	// UpdateStatus updates the status of an ad
	UpdateStatus(ctx context.Context, id AdID, status AdStatus, version int) error
	
	// Delete removes an ad from the repository
	Delete(ctx context.Context, id AdID) error
	
	// Count returns the total number of ads
	Count(ctx context.Context) (int64, error)
	
	// CountByStatus returns the number of ads with a specific status
	CountByStatus(ctx context.Context, status AdStatus) (int64, error)
	
	// CountByPriority returns the number of ads with a specific priority
	CountByPriority(ctx context.Context, priority Priority) (int64, error)
}