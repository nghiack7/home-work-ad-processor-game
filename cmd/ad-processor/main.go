package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"math/rand"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/personal/home-work-ad-process/internal/domain/ad"
	"github.com/personal/home-work-ad-process/internal/domain/queue"
	"github.com/personal/home-work-ad-process/internal/infrastructure/cache"
	"github.com/personal/home-work-ad-process/internal/infrastructure/persistence"
	"github.com/personal/home-work-ad-process/pkg/config"
	mylogger "github.com/personal/home-work-ad-process/pkg/logger"
)

// AdProcessor handles ad processing
type AdProcessor struct {
	adRepo       ad.Repository
	queueManager queue.Manager
	logger       *mylogger.Logger
	config       *config.Config
	workers      []*Worker
	stopChan     chan struct{}
	wg           sync.WaitGroup
}

// Worker represents a processing worker
type Worker struct {
	id           int
	processor    *AdProcessor
	processingCh chan *queue.QueueItem
	stopCh       chan struct{}
}

// NewAdProcessor creates a new AdProcessor
func NewAdProcessor(adRepo ad.Repository, queueManager queue.Manager, logger *mylogger.Logger, config *config.Config) *AdProcessor {
	return &AdProcessor{
		adRepo:       adRepo,
		queueManager: queueManager,
		logger:       logger,
		config:       config,
		stopChan:     make(chan struct{}),
	}
}

// Start starts the ad processor
func (p *AdProcessor) Start(ctx context.Context) error {
	p.logger.Infof("Starting Ad Processor with %d workers", p.config.Queue.WorkerCount)

	// Initialize workers
	p.workers = make([]*Worker, p.config.Queue.WorkerCount)
	for i := 0; i < p.config.Queue.WorkerCount; i++ {
		worker := &Worker{
			id:           i + 1,
			processor:    p,
			processingCh: make(chan *queue.QueueItem, 10),
			stopCh:       make(chan struct{}),
		}
		p.workers[i] = worker

		// Start worker goroutine
		p.wg.Add(1)
		go p.runWorker(ctx, worker)
	}

	// Start queue polling goroutine
	p.wg.Add(1)
	go p.pollQueue(ctx)

	// Start anti-starvation goroutine
	p.wg.Add(1)
	go p.runAntiStarvation(ctx)

	return nil
}

// Stop stops the ad processor
func (p *AdProcessor) Stop() {
	p.logger.Info("Stopping Ad Processor...")

	close(p.stopChan)
	p.wg.Wait()

	p.logger.Info("Ad Processor stopped")
}

// pollQueue continuously polls the queue for new ads to process
func (p *AdProcessor) pollQueue(ctx context.Context) {
	defer p.wg.Done()

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-p.stopChan:
			return
		case <-ticker.C:
			items, err := p.queueManager.DequeueBatch(ctx, p.config.Queue.BatchSize)
			if err != nil {
				p.logger.WithError(err).Error("Failed to dequeue batch")
				continue
			}

			if len(items) == 0 {
				continue
			}

			p.logger.Infof("Dequeued %d items for processing", len(items))

			// Distribute items to workers
			p.distributeItems(items)
		}
	}
}

// distributeItems distributes queue items to available workers
func (p *AdProcessor) distributeItems(items []*queue.QueueItem) {
	for _, item := range items {
		// Find available worker (round-robin)
		for _, worker := range p.workers {
			select {
			case worker.processingCh <- item:
				goto nextItem
			default:
				continue
			}
		}

		// If all workers are busy, wait for one to be available
		select {
		case p.workers[0].processingCh <- item:
		default:
			// Queue is full, skip this item (it will be picked up in next batch)
			p.logger.Warn("All workers busy, skipping item")
		}

	nextItem:
	}
}

// runWorker runs a single worker
func (p *AdProcessor) runWorker(ctx context.Context, worker *Worker) {
	defer p.wg.Done()

	p.logger.Infof("Starting worker %d", worker.id)

	for {
		select {
		case <-ctx.Done():
			return
		case <-worker.stopCh:
			return
		case <-p.stopChan:
			return
		case item := <-worker.processingCh:
			if err := p.processAd(ctx, worker, item); err != nil {
				p.logger.WithError(err).WithField("adId", item.AdID().String()).Error("Failed to process ad")
			}
		}
	}
}

// processAd processes a single ad
func (p *AdProcessor) processAd(ctx context.Context, worker *Worker, item *queue.QueueItem) error {
	startTime := time.Now()
	adID := item.AdID()

	p.logger.WithFields(mylogger.Fields{
		"workerId": worker.id,
		"adId":     adID.String(),
		"priority": item.Priority(),
	}).Info("Starting ad processing")

	// Get ad from repository
	adEntity, err := p.adRepo.FindByID(ctx, adID)
	if err != nil {
		return fmt.Errorf("failed to find ad: %w", err)
	}

	// Start processing
	if err := adEntity.StartProcessing(); err != nil {
		return fmt.Errorf("failed to start processing: %w", err)
	}

	// Save status update
	if err := p.adRepo.Save(ctx, adEntity); err != nil {
		return fmt.Errorf("failed to save ad status: %w", err)
	}

	// Mock processing time (2-5 seconds)
	processingTime := time.Duration(2000+rand.Intn(3000)) * time.Millisecond
	
	select {
	case <-time.After(processingTime):
		// Processing completed successfully
	case <-ctx.Done():
		return ctx.Err()
	}

	// Complete processing
	if err := adEntity.CompleteProcessing(); err != nil {
		return fmt.Errorf("failed to complete processing: %w", err)
	}

	// Save final status
	if err := p.adRepo.Save(ctx, adEntity); err != nil {
		return fmt.Errorf("failed to save final ad status: %w", err)
	}

	duration := time.Since(startTime)
	p.logger.WithFields(mylogger.Fields{
		"workerId":        worker.id,
		"adId":            adID.String(),
		"processingTime":  processingTime,
		"totalDuration":   duration,
		"title":           adEntity.Title(),
		"gameFamily":      adEntity.GameFamily(),
	}).Info("Ad processing completed")

	return nil
}

// runAntiStarvation periodically runs anti-starvation logic
func (p *AdProcessor) runAntiStarvation(ctx context.Context) {
	defer p.wg.Done()

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-p.stopChan:
			return
		case <-ticker.C:
			if err := p.queueManager.ApplyAntiStarvation(ctx); err != nil {
				p.logger.WithError(err).Error("Failed to apply anti-starvation")
			} else {
				p.logger.Debug("Applied anti-starvation logic")
			}
		}
	}
}

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
	myLogger := mylogger.New(cfg.LogLevel, cfg.Environment)

	// Initialize database connection
	db, err := initDatabase(cfg)
	if err != nil {
		myLogger.Fatalf("Failed to initialize database: %v", err)
	}
	defer db.Close()

	// Initialize Redis connection
	redisClient, err := initRedis(cfg)
	if err != nil {
		myLogger.Fatalf("Failed to initialize Redis: %v", err)
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

	// Initialize processor
	processor := NewAdProcessor(adRepo, queueManager, myLogger, cfg)

	// Start processor
	ctx := context.Background()
	if err := processor.Start(ctx); err != nil {
		myLogger.Fatalf("Failed to start processor: %v", err)
	}

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	myLogger.Info("Shutting down processor...")
	processor.Stop()
	myLogger.Info("Processor exited")
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

	// Configure connection pool
	db.SetMaxOpenConns(cfg.Database.MaxOpenConns)
	db.SetMaxIdleConns(cfg.Database.MaxIdleConns)
	db.SetConnMaxLifetime(time.Duration(cfg.Database.ConnMaxLifetimeMinutes) * time.Minute)

	// Test connection
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	return db, nil
}

func initRedis(cfg *config.Config) (*redis.Client, error) {
	client := redis.NewClient(&redis.Options{
		Addr:         fmt.Sprintf("%s:%d", cfg.Redis.Host, cfg.Redis.Port),
		Password:     cfg.Redis.Password,
		DB:           cfg.Redis.DB,
		PoolSize:     cfg.Redis.PoolSize,
		ReadTimeout:  time.Duration(cfg.Redis.ReadTimeoutSeconds) * time.Second,
		WriteTimeout: time.Duration(cfg.Redis.WriteTimeoutSeconds) * time.Second,
	})

	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to ping Redis: %w", err)
	}

	return client, nil
}