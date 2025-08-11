package handlers

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"

	"github.com/personal/home-work-ad-process/internal/application/service"
)

// CommandHandler handles HTTP requests for AI agent command operations
type CommandHandler struct {
	commandService *service.CommandService
	validator      *validator.Validate
}

// NewCommandHandler creates a new CommandHandler
func NewCommandHandler(commandService *service.CommandService) *CommandHandler {
	return &CommandHandler{
		commandService: commandService,
		validator:      validator.New(),
	}
}

// ExecuteCommand handles POST /agent/command
// @Summary Execute natural language command
// @Description Execute a natural language command through the AI agent
// @Tags agent
// @Accept json
// @Produce json
// @Param command body service.ExecuteCommandRequest true "Command data"
// @Success 200 {object} service.ExecuteCommandResponse
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /agent/command [post]
func (h *CommandHandler) ExecuteCommand(c *gin.Context) {
	var req service.ExecuteCommandRequest
	
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "Invalid request format",
			Details: err.Error(),
		})
		return
	}
	
	if err := h.validator.Struct(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "Validation failed",
			Details: err.Error(),
		})
		return
	}
	
	response, err := h.commandService.ExecuteCommand(c.Request.Context(), &req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "Failed to execute command",
			Details: err.Error(),
		})
		return
	}
	
	c.JSON(http.StatusOK, response)
}

// GetCommandStatus handles GET /agent/command/:id
// @Summary Get command execution status
// @Description Get the status and result of a previously executed command
// @Tags agent
// @Produce json
// @Param id path string true "Command ID"
// @Success 200 {object} service.ExecuteCommandResponse
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /agent/command/{id} [get]
func (h *CommandHandler) GetCommandStatus(c *gin.Context) {
	commandID := c.Param("id")
	if commandID == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: "Command ID is required",
		})
		return
	}
	
	response, err := h.commandService.GetCommandStatus(c.Request.Context(), commandID)
	if err != nil {
		if err.Error() == "command not found" {
			c.JSON(http.StatusNotFound, ErrorResponse{
				Error: "Command not found",
			})
			return
		}
		
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "Failed to get command status",
			Details: err.Error(),
		})
		return
	}
	
	c.JSON(http.StatusOK, response)
}

// GetRecentCommands handles GET /agent/commands/recent
// @Summary Get recent commands
// @Description Get a list of recently executed commands for audit purposes
// @Tags agent
// @Produce json
// @Param limit query int false "Number of commands to return" default(10)
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /agent/commands/recent [get]
func (h *CommandHandler) GetRecentCommands(c *gin.Context) {
	limitStr := c.DefaultQuery("limit", "10")
	limit, err := strconv.Atoi(limitStr)
	if err != nil || limit <= 0 || limit > 100 {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: "Limit must be a positive integer between 1 and 100",
		})
		return
	}
	
	commands, err := h.commandService.GetRecentCommands(c.Request.Context(), limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "Failed to get recent commands",
			Details: err.Error(),
		})
		return
	}
	
	response := map[string]interface{}{
		"commands": commands,
		"count":    len(commands),
	}
	
	c.JSON(http.StatusOK, response)
}

// RegisterRoutes registers all command-related routes
func (h *CommandHandler) RegisterRoutes(router *gin.Engine) {
	// API v1 routes
	v1 := router.Group("/api/v1")
	{
		// Agent command operations
		v1.POST("/agent/command", h.ExecuteCommand)
		v1.GET("/agent/command/:id", h.GetCommandStatus)
		v1.GET("/agent/commands/recent", h.GetRecentCommands)
	}
}