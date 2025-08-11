package ad

import (
	"time"

	"github.com/google/uuid"
)

// AdStatus represents the current status of an ad
type AdStatus string

const (
	AdStatusQueued     AdStatus = "queued"
	AdStatusProcessing AdStatus = "processing"
	AdStatusCompleted  AdStatus = "completed"
	AdStatusFailed     AdStatus = "failed"
)

// Priority represents ad priority (1-5, higher is more important)
type Priority int

const (
	PriorityLow    Priority = 1
	PriorityNormal Priority = 3
	PriorityHigh   Priority = 5
)

// IsValid validates the priority value
func (p Priority) IsValid() bool {
	return p >= 1 && p <= 5
}

// Ad represents the core ad entity in our domain
type Ad struct {
	id                   AdID
	title                string
	gameFamily          string
	targetAudience      []string
	priority            Priority
	maxWaitTime         time.Duration
	status              AdStatus
	createdAt           time.Time
	processingStartedAt *time.Time
	processedAt         *time.Time
	queuePosition       *int
	version             int // For optimistic locking
}

// AdID is a value object representing ad identifier
type AdID struct {
	value string
}

// NewAdID creates a new AdID
func NewAdID() AdID {
	return AdID{value: uuid.New().String()}
}

// ParseAdID parses string to AdID
func ParseAdID(id string) (AdID, error) {
	if _, err := uuid.Parse(id); err != nil {
		return AdID{}, err
	}
	return AdID{value: id}, nil
}

// String returns string representation
func (id AdID) String() string {
	return id.value
}

// NewAd creates a new Ad entity (for backward compatibility)
func NewAd(title, gameFamily string, targetAudience []string, priority Priority, maxWaitTime time.Duration) (*Ad, error) {
	factory := NewFactory()
	return factory.CreateAd(title, gameFamily, targetAudience, priority, maxWaitTime)
}

// Getters
func (a *Ad) ID() AdID                     { return a.id }
func (a *Ad) Title() string                { return a.title }
func (a *Ad) GameFamily() string           { return a.gameFamily }
func (a *Ad) TargetAudience() []string     { return a.targetAudience }
func (a *Ad) Priority() Priority           { return a.priority }
func (a *Ad) MaxWaitTime() time.Duration   { return a.maxWaitTime }
func (a *Ad) Status() AdStatus             { return a.status }
func (a *Ad) CreatedAt() time.Time         { return a.createdAt }
func (a *Ad) ProcessingStartedAt() *time.Time { return a.processingStartedAt }
func (a *Ad) ProcessedAt() *time.Time      { return a.processedAt }
func (a *Ad) QueuePosition() *int          { return a.queuePosition }
func (a *Ad) Version() int                 { return a.version }

// Business methods
func (a *Ad) StartProcessing() error {
	if a.status != AdStatusQueued {
		return ErrInvalidStatusTransition
	}
	
	now := time.Now()
	a.status = AdStatusProcessing
	a.processingStartedAt = &now
	a.queuePosition = nil
	a.version++
	
	return nil
}

func (a *Ad) CompleteProcessing() error {
	if a.status != AdStatusProcessing {
		return ErrInvalidStatusTransition
	}
	
	now := time.Now()
	a.status = AdStatusCompleted
	a.processedAt = &now
	a.version++
	
	return nil
}

func (a *Ad) FailProcessing() error {
	if a.status != AdStatusProcessing {
		return ErrInvalidStatusTransition
	}
	
	a.status = AdStatusFailed
	a.version++
	
	return nil
}

func (a *Ad) ChangePriority(newPriority Priority) error {
	if !newPriority.IsValid() {
		return ErrInvalidPriority
	}
	
	if a.status == AdStatusProcessing || a.status == AdStatusCompleted {
		return ErrCannotChangePriorityAfterProcessing
	}
	
	a.priority = newPriority
	a.version++
	
	return nil
}

func (a *Ad) SetQueuePosition(position int) {
	a.queuePosition = &position
}

// WaitTime calculates how long the ad has been waiting
func (a *Ad) WaitTime() time.Duration {
	if a.processingStartedAt != nil {
		return a.processingStartedAt.Sub(a.createdAt)
	}
	return time.Since(a.createdAt)
}

// IsStarving checks if the ad should receive priority boost due to long wait time
func (a *Ad) IsStarving() bool {
	return a.status == AdStatusQueued && a.WaitTime() > a.maxWaitTime
}

// CalculateEffectivePriority calculates priority with anti-starvation boost
func (a *Ad) CalculateEffectivePriority(antiStarvationEnabled bool) Priority {
	if !antiStarvationEnabled {
		return a.priority
	}
	
	if a.IsStarving() {
		// Boost priority based on how long it's been waiting
		waitOverrun := a.WaitTime() - a.maxWaitTime
		boost := int(waitOverrun.Minutes() / 5) // +1 priority every 5 minutes overrun
		
		effectivePriority := int(a.priority) + boost
		if effectivePriority > int(PriorityHigh) {
			effectivePriority = int(PriorityHigh)
		}
		
		return Priority(effectivePriority)
	}
	
	return a.priority
}