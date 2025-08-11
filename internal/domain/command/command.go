package command

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/personal/home-work-ad-process/internal/domain/ad"
)

// CommandType represents the type of command
type CommandType string

const (
	CommandTypeQueueModification  CommandType = "queue_modification"
	CommandTypeSystemConfiguration CommandType = "system_configuration"
	CommandTypeStatusQuery        CommandType = "status_query"
	CommandTypeAnalytics         CommandType = "analytics"
)

// CommandStatus represents the execution status of a command
type CommandStatus string

const (
	CommandStatusPending   CommandStatus = "pending"
	CommandStatusExecuting CommandStatus = "executing"
	CommandStatusExecuted  CommandStatus = "executed"
	CommandStatusFailed    CommandStatus = "failed"
	CommandStatusInvalid   CommandStatus = "invalid"
)

// CommandID is a value object for command identification
type CommandID struct {
	value string
}

// NewCommandID creates a new CommandID
func NewCommandID() CommandID {
	return CommandID{value: uuid.New().String()}
}

// String returns string representation
func (id CommandID) String() string {
	return id.value
}

// Command represents an AI-parsed command to be executed
type Command struct {
	id            CommandID
	originalText  string
	commandType   CommandType
	intent        string
	parameters    map[string]interface{}
	status        CommandStatus
	result        interface{}
	error         string
	createdAt     time.Time
	executedAt    *time.Time
	executionTime *time.Duration
}

// NewCommand creates a new command
func NewCommand(originalText string, commandType CommandType, intent string, parameters map[string]interface{}) *Command {
	return &Command{
		id:           NewCommandID(),
		originalText: originalText,
		commandType:  commandType,
		intent:       intent,
		parameters:   parameters,
		status:       CommandStatusPending,
		createdAt:    time.Now(),
	}
}

// Getters
func (c *Command) ID() CommandID                           { return c.id }
func (c *Command) OriginalText() string                    { return c.originalText }
func (c *Command) Type() CommandType                       { return c.commandType }
func (c *Command) Intent() string                          { return c.intent }
func (c *Command) Parameters() map[string]interface{}      { return c.parameters }
func (c *Command) Status() CommandStatus                   { return c.status }
func (c *Command) Result() interface{}                     { return c.result }
func (c *Command) Error() string                          { return c.error }
func (c *Command) CreatedAt() time.Time                   { return c.createdAt }
func (c *Command) ExecutedAt() *time.Time                  { return c.executedAt }
func (c *Command) ExecutionTime() *time.Duration          { return c.executionTime }

// Business methods
func (c *Command) StartExecution() {
	c.status = CommandStatusExecuting
}

func (c *Command) CompleteExecution(result interface{}) {
	now := time.Now()
	executionTime := now.Sub(c.createdAt)
	
	c.status = CommandStatusExecuted
	c.result = result
	c.executedAt = &now
	c.executionTime = &executionTime
}

func (c *Command) FailExecution(errorMsg string) {
	now := time.Now()
	executionTime := now.Sub(c.createdAt)
	
	c.status = CommandStatusFailed
	c.error = errorMsg
	c.executedAt = &now
	c.executionTime = &executionTime
}

func (c *Command) MarkInvalid(reason string) {
	c.status = CommandStatusInvalid
	c.error = reason
}

// GetParameter safely gets a parameter with type assertion
func (c *Command) GetParameter(key string) (interface{}, bool) {
	value, exists := c.parameters[key]
	return value, exists
}

// GetStringParameter gets a string parameter
func (c *Command) GetStringParameter(key string) (string, bool) {
	if value, exists := c.parameters[key]; exists {
		if str, ok := value.(string); ok {
			return str, true
		}
	}
	return "", false
}

// GetIntParameter gets an integer parameter
func (c *Command) GetIntParameter(key string) (int, bool) {
	if value, exists := c.parameters[key]; exists {
		if i, ok := value.(int); ok {
			return i, true
		}
		if f, ok := value.(float64); ok {
			return int(f), true
		}
	}
	return 0, false
}

// GetPriorityParameter gets a priority parameter
func (c *Command) GetPriorityParameter(key string) (ad.Priority, bool) {
	if i, exists := c.GetIntParameter(key); exists {
		priority := ad.Priority(i)
		if priority.IsValid() {
			return priority, true
		}
	}
	return 0, false
}

// Parser defines the interface for parsing natural language commands
type Parser interface {
	// Parse parses natural language text into a structured command
	Parse(ctx context.Context, text string) (*Command, error)
	
	// ValidateCommand validates that a command has all required parameters
	ValidateCommand(ctx context.Context, cmd *Command) error
}

// Executor defines the interface for executing parsed commands
type Executor interface {
	// Execute executes a validated command
	Execute(ctx context.Context, cmd *Command) error
	
	// CanExecute checks if a command can be executed
	CanExecute(ctx context.Context, cmd *Command) bool
}

// Repository defines the interface for command persistence
type Repository interface {
	// Save saves a command
	Save(ctx context.Context, cmd *Command) error
	
	// FindByID finds a command by ID
	FindByID(ctx context.Context, id CommandID) (*Command, error)
	
	// FindRecent finds recent commands
	FindRecent(ctx context.Context, limit int) ([]*Command, error)
	
	// FindByStatus finds commands by status
	FindByStatus(ctx context.Context, status CommandStatus) ([]*Command, error)
}