package handlers

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"

	"github.com/personal/home-work-ad-process/internal/application/service"
)

// AdHandler handles HTTP requests for ad operations
type AdHandler struct {
	adService *service.AdService
	validator *validator.Validate
}

// NewAdHandler creates a new AdHandler
func NewAdHandler(adService *service.AdService) *AdHandler {
	return &AdHandler{
		adService: adService,
		validator: validator.New(),
	}
}

// ErrorResponse represents an error response
type ErrorResponse struct {
	Error   string `json:"error"`
	Code    string `json:"code,omitempty"`
	Details string `json:"details,omitempty"`
}

// CreateAd handles POST /ads
// @Summary Create a new ad
// @Description Create a new ad and add it to the processing queue
// @Tags ads
// @Accept json
// @Produce json
// @Param ad body service.CreateAdRequest true "Ad data"
// @Success 201 {object} service.CreateAdResponse
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /ads [post]
func (h *AdHandler) CreateAd(c *gin.Context) {
	var req service.CreateAdRequest
	
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
	
	response, err := h.adService.CreateAd(c.Request.Context(), &req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "Failed to create ad",
			Details: err.Error(),
		})
		return
	}
	
	c.JSON(http.StatusCreated, response)
}

// GetAdStatus handles GET /ads/:id
// @Summary Get ad status
// @Description Get the current status and details of an ad
// @Tags ads
// @Produce json
// @Param id path string true "Ad ID"
// @Success 200 {object} service.GetAdStatusResponse
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /ads/{id} [get]
func (h *AdHandler) GetAdStatus(c *gin.Context) {
	adID := c.Param("id")
	if adID == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: "Ad ID is required",
		})
		return
	}
	
	response, err := h.adService.GetAdStatus(c.Request.Context(), adID)
	if err != nil {
		// Check if it's a not found error
		if err.Error() == "ad not found" {
			c.JSON(http.StatusNotFound, ErrorResponse{
				Error: "Ad not found",
			})
			return
		}
		
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "Failed to get ad status",
			Details: err.Error(),
		})
		return
	}
	
	c.JSON(http.StatusOK, response)
}

// GetQueueStats handles GET /ads/queue/stats
// @Summary Get queue statistics
// @Description Get current queue distribution and statistics
// @Tags ads
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Failure 500 {object} ErrorResponse
// @Router /ads/queue/stats [get]
func (h *AdHandler) GetQueueStats(c *gin.Context) {
	distribution, err := h.adService.GetQueueDistribution(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "Failed to get queue statistics",
			Details: err.Error(),
		})
		return
	}
	
	// Convert to a more user-friendly format
	stats := make(map[string]interface{})
	stats["distribution"] = distribution
	
	var total int64
	for _, count := range distribution {
		total += count
	}
	stats["total"] = total
	
	c.JSON(http.StatusOK, stats)
}

// GetNextAds handles GET /ads/queue/next
// @Summary Get next ads to be processed
// @Description Get the next N ads that would be processed from the queue
// @Tags ads
// @Produce json
// @Param count query int false "Number of ads to return" default(5)
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /ads/queue/next [get]
func (h *AdHandler) GetNextAds(c *gin.Context) {
	countStr := c.DefaultQuery("count", "5")
	count, err := strconv.Atoi(countStr)
	if err != nil || count <= 0 || count > 100 {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: "Count must be a positive integer between 1 and 100",
		})
		return
	}
	
	nextAds, err := h.adService.GetNextAds(c.Request.Context(), count)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "Failed to get next ads",
			Details: err.Error(),
		})
		return
	}
	
	response := map[string]interface{}{
		"nextAds": nextAds,
		"count":   len(nextAds),
	}
	
	c.JSON(http.StatusOK, response)
}

// HealthCheck handles GET /health
// @Summary Health check
// @Description Check if the service is healthy
// @Tags health
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /health [get]
func (h *AdHandler) HealthCheck(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status":    "healthy",
		"service":   "ad-api",
		"timestamp": c.Request.Header.Get("X-Request-Time"),
	})
}

// ReadinessCheck handles GET /ready
// @Summary Readiness check
// @Description Check if the service is ready to handle requests
// @Tags health
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Failure 503 {object} ErrorResponse
// @Router /ready [get]
func (h *AdHandler) ReadinessCheck(c *gin.Context) {
	// In a real implementation, you'd check dependencies like database connectivity
	// For now, we'll just return ready
	c.JSON(http.StatusOK, gin.H{
		"status":  "ready",
		"service": "ad-api",
	})
}

// RegisterRoutes registers all ad-related routes
func (h *AdHandler) RegisterRoutes(router *gin.Engine) {
	// Health checks
	router.GET("/health", h.HealthCheck)
	router.GET("/ready", h.ReadinessCheck)
	
	// API v1 routes
	v1 := router.Group("/api/v1")
	{
		// Ad operations
		v1.POST("/ads", h.CreateAd)
		v1.GET("/ads/:id", h.GetAdStatus)
		
		// Queue operations
		v1.GET("/ads/queue/stats", h.GetQueueStats)
		v1.GET("/ads/queue/next", h.GetNextAds)
	}
}