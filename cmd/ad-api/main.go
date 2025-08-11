package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"

	"github.com/personal/home-work-ad-process/internal/application/service"
	"github.com/personal/home-work-ad-process/internal/domain/command"
	"github.com/personal/home-work-ad-process/internal/domain/queue"
	"github.com/personal/home-work-ad-process/internal/infrastructure/cache"
	"github.com/personal/home-work-ad-process/internal/infrastructure/external"
	"github.com/personal/home-work-ad-process/internal/infrastructure/persistence"
	"github.com/personal/home-work-ad-process/internal/interfaces/http/handlers"
	"github.com/personal/home-work-ad-process/pkg/config"
	"github.com/personal/home-work-ad-process/pkg/logger"
	"github.com/personal/home-work-ad-process/pkg/monitoring"
)

func main() {
	// Handle health check command
	if len(os.Args) > 1 && os.Args[1] == "-health-check" {
		os.Exit(0) // Simple health check - just exit successfully
	}

	// Initialize configuration
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// Initialize logger
	logger := logger.New(cfg.LogLevel, cfg.Environment)

	// Initialize database connection
	db, err := initDatabase(cfg)
	if err != nil {
		logger.Fatalf("Failed to initialize database: %v", err)
	}
	defer db.Close()

	// Initialize Redis connection
	redisClient, err := initRedis(cfg)
	if err != nil {
		logger.Fatalf("Failed to initialize Redis: %v", err)
	}
	defer redisClient.Close()

	// Initialize repositories
	adRepo := persistence.NewPostgresAdRepository(db)

	// Initialize queue manager
	queueConfig := &queue.QueueConfig{
		AntiStarvationEnabled: cfg.Queue.AntiStarvationEnabled,
		MaxWaitTime:          time.Duration(cfg.Queue.MaxWaitTimeSeconds) * time.Second,
		WorkerCount:          cfg.Queue.WorkerCount,
		BatchSize:            cfg.Queue.BatchSize,
		ProcessingTimeout:    time.Duration(cfg.Queue.ProcessingTimeoutSeconds) * time.Second,
	}
	queueManager := cache.NewRedisQueueManager(redisClient, queueConfig, cfg.Queue.ShardCount)

	// Initialize command infrastructure
	var commandParser command.Parser
	if cfg.AIAgent.GoogleADK.APIKey != "" {
		// Use Google ADK for production
		commandParser = external.NewGoogleADKCommandParser(cfg.AIAgent.GoogleADK.APIKey)
		logger.Info("Using Google ADK command parser")
	} else {
		// Fallback to mock parser for development
		commandParser = external.NewMockCommandParser()
		logger.Warn("Using mock command parser - set GOOGLE_AI_API_KEY for production")
	}
	commandRepo := persistence.NewMemoryCommandRepository()
	
	// Initialize services
	adService := service.NewAdService(adRepo, queueManager)
	commandExecutor := external.NewMockCommandExecutor(adService)
	commandService := service.NewCommandService(commandParser, commandExecutor, commandRepo, adService)

	// Initialize handlers
	adHandler := handlers.NewAdHandler(adService)
	commandHandler := handlers.NewCommandHandler(commandService)

	// Setup HTTP server
	router := setupRouter(cfg, logger)
	adHandler.RegisterRoutes(router)
	commandHandler.RegisterRoutes(router)
	
	// Add metrics endpoint
	router.GET("/metrics", gin.WrapH(monitoring.PrometheusHandler()))

	// High-performance HTTP server configuration for 1M RPS
	server := &http.Server{
		Addr:           fmt.Sprintf(":%d", cfg.Server.Port),
		Handler:        router,
		ReadTimeout:    time.Duration(cfg.Server.ReadTimeoutSeconds) * time.Second,
		WriteTimeout:   time.Duration(cfg.Server.WriteTimeoutSeconds) * time.Second,
		IdleTimeout:    time.Duration(cfg.Server.IdleTimeoutSeconds) * time.Second,
		MaxHeaderBytes: 1 << 20, // 1MB max header size
	}

	// Start server in a goroutine
	go func() {
		logger.Infof("Starting Ad API server on port %d", cfg.Server.Port)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatalf("Failed to start server: %v", err)
		}
	}()

	// Wait for interrupt signal to gracefully shutdown the server
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info("Shutting down server...")

	// Give outstanding requests 30 seconds to complete
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		logger.Fatalf("Server forced to shutdown: %v", err)
	}

	logger.Info("Server exited")
}

func initDatabase(cfg *config.Config) (*sql.DB, error) {
	dsn := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		cfg.Database.Host,
		cfg.Database.Port,
		cfg.Database.User,
		cfg.Database.Password,
		cfg.Database.Name,
		cfg.Database.SSLMode,
	)

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open database connection: %w", err)
	}

	// High-performance connection pool for 1M RPS
	maxOpenConns := cfg.Database.MaxOpenConns
	if maxOpenConns == 0 {
		maxOpenConns = 200 // Default for high-throughput
	}
	maxIdleConns := cfg.Database.MaxIdleConns
	if maxIdleConns == 0 {
		maxIdleConns = 50 // Default for high-throughput
	}
	
	db.SetMaxOpenConns(maxOpenConns)
	db.SetMaxIdleConns(maxIdleConns)
	db.SetConnMaxLifetime(time.Duration(cfg.Database.ConnMaxLifetimeMinutes) * time.Minute)
	db.SetConnMaxIdleTime(15 * time.Minute) // Aggressive idle timeout for better resource management

	// Test connection
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	return db, nil
}

func initRedis(cfg *config.Config) (*redis.Client, error) {
	// High-performance Redis configuration for 1M RPS
	poolSize := cfg.Redis.PoolSize
	if poolSize == 0 {
		poolSize = 1000 // Large connection pool for high throughput
	}
	
	client := redis.NewClient(&redis.Options{
		Addr:            fmt.Sprintf("%s:%d", cfg.Redis.Host, cfg.Redis.Port),
		Password:        cfg.Redis.Password,
		DB:              cfg.Redis.DB,
		PoolSize:        poolSize,
		MinIdleConns:    100, // Keep connections warm
		MaxIdleConns:    200, // Allow more idle connections
		ConnMaxIdleTime: 10 * time.Minute,
		ConnMaxLifetime: 1 * time.Hour,
		ReadTimeout:     time.Duration(cfg.Redis.ReadTimeoutSeconds) * time.Second,
		WriteTimeout:    time.Duration(cfg.Redis.WriteTimeoutSeconds) * time.Second,
		DialTimeout:     5 * time.Second,
		PoolTimeout:     30 * time.Second,
	})

	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to ping Redis: %w", err)
	}

	return client, nil
}

func setupRouter(cfg *config.Config, logger *logger.Logger) *gin.Engine {
	if cfg.Environment == "production" {
		gin.SetMode(gin.ReleaseMode)
	}

	router := gin.New()

	// Middleware
	router.Use(gin.Recovery())
	router.Use(corsMiddleware())
	router.Use(loggingMiddleware(logger))
	router.Use(monitoring.MetricsMiddleware())
	router.Use(metricsMiddleware())

	return router
}

func corsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Credentials", "true")
		c.Header("Access-Control-Allow-Headers", "Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization, accept, origin, Cache-Control, X-Requested-With")
		c.Header("Access-Control-Allow-Methods", "POST, GET, OPTIONS, PUT, DELETE")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	}
}

func loggingMiddleware(logger *logger.Logger) gin.HandlerFunc {
	return gin.LoggerWithConfig(gin.LoggerConfig{
		Output: logger.Writer(),
		Formatter: func(param gin.LogFormatterParams) string {
			return fmt.Sprintf("[%s] %s %s %d %s %s\n",
				param.TimeStamp.Format("2006-01-02 15:04:05"),
				param.Method,
				param.Path,
				param.StatusCode,
				param.Latency,
				param.ClientIP,
			)
		},
	})
}

func metricsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()

		c.Next()

		duration := time.Since(start)
		
		// TODO: Add Prometheus metrics here
		_ = duration
	}
}