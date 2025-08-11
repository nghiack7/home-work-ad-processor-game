package ad

import (
	"time"
)

// Factory provides methods to create and reconstruct Ad entities
type Factory struct{}

// NewFactory creates a new Ad factory
func NewFactory() *Factory {
	return &Factory{}
}

// CreateAd creates a new Ad entity from business requirements
func (f *Factory) CreateAd(title, gameFamily string, targetAudience []string, priority Priority, maxWaitTime time.Duration) (*Ad, error) {
	if title == "" {
		return nil, ErrInvalidTitle
	}
	if gameFamily == "" {
		return nil, ErrInvalidGameFamily
	}
	if len(targetAudience) == 0 {
		return nil, ErrInvalidTargetAudience
	}
	if !priority.IsValid() {
		return nil, ErrInvalidPriority
	}

	return &Ad{
		id:             NewAdID(),
		title:          title,
		gameFamily:     gameFamily,
		targetAudience: targetAudience,
		priority:       priority,
		maxWaitTime:    maxWaitTime,
		status:         AdStatusQueued,
		createdAt:      time.Now(),
		version:        1,
	}, nil
}

// ReconstructAd reconstructs an Ad entity from persistence data
func (f *Factory) ReconstructAd(
	id AdID,
	title, gameFamily string,
	targetAudience []string,
	priority Priority,
	maxWaitTime time.Duration,
	status AdStatus,
	createdAt time.Time,
	processingStartedAt *time.Time,
	processedAt *time.Time,
	version int,
) *Ad {
	return &Ad{
		id:                   id,
		title:                title,
		gameFamily:          gameFamily,
		targetAudience:      targetAudience,
		priority:            priority,
		maxWaitTime:         maxWaitTime,
		status:              status,
		createdAt:           createdAt,
		processingStartedAt: processingStartedAt,
		processedAt:         processedAt,
		version:             version,
	}
}