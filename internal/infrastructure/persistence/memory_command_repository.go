package persistence

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/personal/home-work-ad-process/internal/domain/command"
)

// MemoryCommandRepository implements command.Repository using in-memory storage
type MemoryCommandRepository struct {
	commands map[string]*command.Command
	mutex    sync.RWMutex
}

// NewMemoryCommandRepository creates a new in-memory command repository
func NewMemoryCommandRepository() *MemoryCommandRepository {
	return &MemoryCommandRepository{
		commands: make(map[string]*command.Command),
	}
}

// Save saves a command
func (r *MemoryCommandRepository) Save(ctx context.Context, cmd *command.Command) error {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	
	r.commands[cmd.ID().String()] = cmd
	return nil
}

// FindByID finds a command by ID
func (r *MemoryCommandRepository) FindByID(ctx context.Context, id command.CommandID) (*command.Command, error) {
	r.mutex.RLock()
	defer r.mutex.RUnlock()
	
	cmd, exists := r.commands[id.String()]
	if !exists {
		return nil, command.ErrCommandNotFound
	}
	
	return cmd, nil
}

// FindRecent finds recent commands
func (r *MemoryCommandRepository) FindRecent(ctx context.Context, limit int) ([]*command.Command, error) {
	r.mutex.RLock()
	defer r.mutex.RUnlock()
	
	// Convert map to slice
	commands := make([]*command.Command, 0, len(r.commands))
	for _, cmd := range r.commands {
		commands = append(commands, cmd)
	}
	
	// Sort by creation time (most recent first)
	sort.Slice(commands, func(i, j int) bool {
		return commands[i].CreatedAt().After(commands[j].CreatedAt())
	})
	
	// Limit results
	if limit > 0 && limit < len(commands) {
		commands = commands[:limit]
	}
	
	return commands, nil
}

// FindByStatus finds commands by status
func (r *MemoryCommandRepository) FindByStatus(ctx context.Context, status command.CommandStatus) ([]*command.Command, error) {
	r.mutex.RLock()
	defer r.mutex.RUnlock()
	
	var result []*command.Command
	for _, cmd := range r.commands {
		if cmd.Status() == status {
			result = append(result, cmd)
		}
	}
	
	// Sort by creation time
	sort.Slice(result, func(i, j int) bool {
		return result[i].CreatedAt().After(result[j].CreatedAt())
	})
	
	return result, nil
}

// CleanupOldCommands removes commands older than the specified duration
func (r *MemoryCommandRepository) CleanupOldCommands(maxAge time.Duration) int {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	
	threshold := time.Now().Add(-maxAge)
	count := 0
	
	for id, cmd := range r.commands {
		if cmd.CreatedAt().Before(threshold) {
			delete(r.commands, id)
			count++
		}
	}
	
	return count
}