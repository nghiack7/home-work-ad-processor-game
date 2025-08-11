package service

import (
	"context"
	"fmt"

	"github.com/personal/home-work-ad-process/internal/domain/command"
)

// CommandService provides business logic for AI command operations
type CommandService struct {
	parser     command.Parser
	executor   command.Executor
	repository command.Repository
	adService  *AdService
}

// NewCommandService creates a new CommandService
func NewCommandService(
	parser command.Parser,
	executor command.Executor,
	repository command.Repository,
	adService *AdService,
) *CommandService {
	return &CommandService{
		parser:     parser,
		executor:   executor,
		repository: repository,
		adService:  adService,
	}
}

// ExecuteCommandRequest represents a request to execute a natural language command
type ExecuteCommandRequest struct {
	Command string `json:"command" validate:"required"`
}

// ExecuteCommandResponse represents the response after executing a command
type ExecuteCommandResponse struct {
	CommandID     string      `json:"commandId"`
	Status        string      `json:"status"`
	Result        interface{} `json:"result,omitempty"`
	Error         string      `json:"error,omitempty"`
	ExecutionTime string      `json:"executionTime"`
}

// ExecuteCommand parses and executes a natural language command
func (s *CommandService) ExecuteCommand(ctx context.Context, req *ExecuteCommandRequest) (*ExecuteCommandResponse, error) {
	// Parse the natural language command
	cmd, err := s.parser.Parse(ctx, req.Command)
	if err != nil {
		return &ExecuteCommandResponse{
			CommandID: "",
			Status:    string(command.CommandStatusInvalid),
			Error:     fmt.Sprintf("Failed to parse command: %v", err),
		}, nil
	}
	
	// Save the command for audit trail
	if err := s.repository.Save(ctx, cmd); err != nil {
		// Log error but continue execution
	}
	
	// Validate the command
	if err := s.parser.ValidateCommand(ctx, cmd); err != nil {
		cmd.MarkInvalid(err.Error())
		s.repository.Save(ctx, cmd) // Update status
		
		return &ExecuteCommandResponse{
			CommandID: cmd.ID().String(),
			Status:    string(cmd.Status()),
			Error:     err.Error(),
		}, nil
	}
	
	// Check if command can be executed
	if !s.executor.CanExecute(ctx, cmd) {
		cmd.MarkInvalid("Command cannot be executed")
		s.repository.Save(ctx, cmd)
		
		return &ExecuteCommandResponse{
			CommandID: cmd.ID().String(),
			Status:    string(cmd.Status()),
			Error:     "Command cannot be executed",
		}, nil
	}
	
	// Execute the command
	cmd.StartExecution()
	s.repository.Save(ctx, cmd)
	
	if err := s.executor.Execute(ctx, cmd); err != nil {
		cmd.FailExecution(err.Error())
		s.repository.Save(ctx, cmd)
		
		return &ExecuteCommandResponse{
			CommandID:     cmd.ID().String(),
			Status:        string(cmd.Status()),
			Error:         err.Error(),
			ExecutionTime: cmd.ExecutionTime().String(),
		}, nil
	}
	
	// Command executed successfully
	s.repository.Save(ctx, cmd)
	
	executionTime := ""
	if cmd.ExecutionTime() != nil {
		executionTime = cmd.ExecutionTime().String()
	}
	
	return &ExecuteCommandResponse{
		CommandID:     cmd.ID().String(),
		Status:        string(cmd.Status()),
		Result:        cmd.Result(),
		ExecutionTime: executionTime,
	}, nil
}

// GetCommandStatus returns the status of a previously executed command
func (s *CommandService) GetCommandStatus(ctx context.Context, commandID string) (*ExecuteCommandResponse, error) {
	cmdID := command.CommandID{}
	// Note: We need to implement ParseCommandID similar to ad.ParseAdID
	
	cmd, err := s.repository.FindByID(ctx, cmdID)
	if err != nil {
		return nil, fmt.Errorf("command not found: %w", err)
	}
	
	executionTime := ""
	if cmd.ExecutionTime() != nil {
		executionTime = cmd.ExecutionTime().String()
	}
	
	response := &ExecuteCommandResponse{
		CommandID:     cmd.ID().String(),
		Status:        string(cmd.Status()),
		Result:        cmd.Result(),
		ExecutionTime: executionTime,
	}
	
	if cmd.Error() != "" {
		response.Error = cmd.Error()
	}
	
	return response, nil
}

// GetRecentCommands returns recent commands for audit/debugging
func (s *CommandService) GetRecentCommands(ctx context.Context, limit int) ([]*command.Command, error) {
	return s.repository.FindRecent(ctx, limit)
}