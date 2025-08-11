package ad

import "errors"

// Domain errors for Ad aggregate
var (
	ErrInvalidTitle                         = errors.New("ad title cannot be empty")
	ErrInvalidGameFamily                   = errors.New("game family cannot be empty")
	ErrInvalidTargetAudience               = errors.New("target audience cannot be empty")
	ErrInvalidPriority                     = errors.New("priority must be between 1 and 5")
	ErrInvalidStatusTransition             = errors.New("invalid status transition")
	ErrCannotChangePriorityAfterProcessing = errors.New("cannot change priority after processing started")
	ErrAdNotFound                          = errors.New("ad not found")
	ErrAdAlreadyExists                     = errors.New("ad already exists")
	ErrOptimisticLockFailed                = errors.New("optimistic lock failed - ad was modified by another process")
)