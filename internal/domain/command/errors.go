package command

import "errors"

// Domain errors for Command aggregate
var (
	ErrCommandNotFound        = errors.New("command not found")
	ErrInvalidCommand         = errors.New("invalid command")
	ErrCommandAlreadyExecuted = errors.New("command already executed")
	ErrCommandExecutionFailed = errors.New("command execution failed")
	ErrUnsupportedIntent      = errors.New("unsupported command intent")
	ErrMissingParameter       = errors.New("missing required parameter")
	ErrInvalidParameter       = errors.New("invalid parameter value")
)