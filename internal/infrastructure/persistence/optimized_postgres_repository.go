package persistence

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/lib/pq"
	"github.com/personal/home-work-ad-process/internal/domain/ad"
)

// OptimizedPostgresAdRepository is a high-performance PostgreSQL repository optimized for 1M+ RPS
type OptimizedPostgresAdRepository struct {
	db           *sql.DB
	readReplicas []*sql.DB
	writeStmt    *sql.Stmt
	readStmt     *sql.Stmt
	batchStmt    *sql.Stmt
	updateStmt   *sql.Stmt
	shardCount   int
}

// ConnectionPoolConfig holds database connection pool configuration
type ConnectionPoolConfig struct {
	MaxOpenConns        int
	MaxIdleConns        int
	ConnMaxLifetime     time.Duration
	ConnMaxIdleTime     time.Duration
	ReadReplicaCount    int
	ShardCount          int
	PreparedStatements  bool
	BatchSize           int
}

// NewOptimizedPostgresAdRepository creates a new optimized repository
func NewOptimizedPostgresAdRepository(db *sql.DB, readReplicas []*sql.DB, config *ConnectionPoolConfig) (*OptimizedPostgresAdRepository, error) {
	repo := &OptimizedPostgresAdRepository{
		db:           db,
		readReplicas: readReplicas,
		shardCount:   config.ShardCount,
	}

	// Optimize connection pools
	if err := repo.optimizeConnectionPools(config); err != nil {
		return nil, fmt.Errorf("failed to optimize connection pools: %w", err)
	}

	// Prepare statements for better performance
	if config.PreparedStatements {
		if err := repo.prepareStatements(); err != nil {
			return nil, fmt.Errorf("failed to prepare statements: %w", err)
		}
	}

	return repo, nil
}

// optimizeConnectionPools configures database connection pools for high performance
func (r *OptimizedPostgresAdRepository) optimizeConnectionPools(config *ConnectionPoolConfig) error {
	// Configure primary database
	r.db.SetMaxOpenConns(config.MaxOpenConns)
	r.db.SetMaxIdleConns(config.MaxIdleConns)
	r.db.SetConnMaxLifetime(config.ConnMaxLifetime)
	r.db.SetConnMaxIdleTime(config.ConnMaxIdleTime)

	// Configure read replicas
	for _, replica := range r.readReplicas {
		replica.SetMaxOpenConns(config.MaxOpenConns / 2) // Distribute connections
		replica.SetMaxIdleConns(config.MaxIdleConns / 2)
		replica.SetConnMaxLifetime(config.ConnMaxLifetime)
		replica.SetConnMaxIdleTime(config.ConnMaxIdleTime)
	}

	return nil
}

// prepareStatements prepares commonly used SQL statements
func (r *OptimizedPostgresAdRepository) prepareStatements() error {
	var err error

	// Prepare write statement
	r.writeStmt, err = r.db.Prepare(`
		INSERT INTO ads (
			ad_id, title, game_family, target_audience, priority, 
			created_at, max_wait_time, status, shard_key
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT (ad_id) DO UPDATE SET
			title = EXCLUDED.title,
			game_family = EXCLUDED.game_family,
			target_audience = EXCLUDED.target_audience,
			priority = EXCLUDED.priority,
			max_wait_time = EXCLUDED.max_wait_time,
			status = EXCLUDED.status
	`)
	if err != nil {
		return fmt.Errorf("failed to prepare write statement: %w", err)
	}

	// Prepare read statement
	r.readStmt, err = r.getReadDB().Prepare(`
		SELECT ad_id, title, game_family, target_audience, priority, 
			   created_at, max_wait_time, status, processing_started_at, processed_at
		FROM ads WHERE ad_id = $1
	`)
	if err != nil {
		return fmt.Errorf("failed to prepare read statement: %w", err)
	}

	// Prepare batch read statement
	r.batchStmt, err = r.getReadDB().Prepare(`
		SELECT ad_id, title, game_family, target_audience, priority, 
			   created_at, max_wait_time, status, processing_started_at, processed_at
		FROM ads WHERE ad_id = ANY($1)
	`)
	if err != nil {
		return fmt.Errorf("failed to prepare batch statement: %w", err)
	}

	// Prepare update statement
	r.updateStmt, err = r.db.Prepare(`
		UPDATE ads SET status = $2, processing_started_at = $3, processed_at = $4 
		WHERE ad_id = $1
	`)
	if err != nil {
		return fmt.Errorf("failed to prepare update statement: %w", err)
	}

	return nil
}

// getReadDB returns a read replica database connection using round-robin
func (r *OptimizedPostgresAdRepository) getReadDB() *sql.DB {
	if len(r.readReplicas) == 0 {
		return r.db // Fallback to primary
	}

	// Simple round-robin selection
	// In production, use more sophisticated load balancing
	index := int(time.Now().UnixNano()) % len(r.readReplicas)
	return r.readReplicas[index]
}

// getShardKey calculates shard key for an ad
func (r *OptimizedPostgresAdRepository) getShardKey(adID ad.AdID) string {
	if r.shardCount <= 1 {
		return "0"
	}

	hash := 0
	for _, c := range adID.String() {
		hash = int(c) + ((hash << 5) - hash)
	}
	if hash < 0 {
		hash = -hash
	}

	shard := hash % r.shardCount
	return fmt.Sprintf("%d", shard)
}

// Save stores an ad in the database with optimal performance
func (r *OptimizedPostgresAdRepository) Save(ctx context.Context, adEntity *ad.Ad) error {
	shardKey := r.getShardKey(adEntity.ID())

	// Use prepared statement if available
	if r.writeStmt != nil {
		_, err := r.writeStmt.ExecContext(
			ctx,
			adEntity.ID().String(),
			adEntity.Title(),
			adEntity.GameFamily(),
			adEntity.TargetAudience(),
			int(adEntity.Priority()),
			adEntity.CreatedAt(),
			int(adEntity.MaxWaitTime().Seconds()),
			adEntity.Status(),
			shardKey,
		)
		return err
	}

	// Fallback to direct query
	query := `
		INSERT INTO ads (
			ad_id, title, game_family, target_audience, priority, 
			created_at, max_wait_time, status, shard_key
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT (ad_id) DO UPDATE SET
			title = EXCLUDED.title,
			game_family = EXCLUDED.game_family,
			target_audience = EXCLUDED.target_audience,
			priority = EXCLUDED.priority,
			max_wait_time = EXCLUDED.max_wait_time,
			status = EXCLUDED.status
	`

	_, err := r.db.ExecContext(
		ctx,
		query,
		adEntity.ID().String(),
		adEntity.Title(),
		adEntity.GameFamily(),
		adEntity.TargetAudience(),
		int(adEntity.Priority()),
		adEntity.CreatedAt(),
		int(adEntity.MaxWaitTime().Seconds()),
		adEntity.Status(),
		shardKey,
	)

	return err
}

// SaveBatch stores multiple ads in a single transaction for better performance
func (r *OptimizedPostgresAdRepository) SaveBatch(ctx context.Context, ads []*ad.Ad) error {
	if len(ads) == 0 {
		return nil
	}

	// Begin transaction
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Prepare batch insert statement
	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO ads (
			ad_id, title, game_family, target_audience, priority, 
			created_at, max_wait_time, status, shard_key
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT (ad_id) DO UPDATE SET
			title = EXCLUDED.title,
			game_family = EXCLUDED.game_family,
			target_audience = EXCLUDED.target_audience,
			priority = EXCLUDED.priority,
			max_wait_time = EXCLUDED.max_wait_time,
			status = EXCLUDED.status
	`)
	if err != nil {
		return fmt.Errorf("failed to prepare batch statement: %w", err)
	}
	defer stmt.Close()

	// Execute batch
	for _, adEntity := range ads {
		shardKey := r.getShardKey(adEntity.ID())
		_, err := stmt.ExecContext(
			ctx,
			adEntity.ID().String(),
			adEntity.Title(),
			adEntity.GameFamily(),
			adEntity.TargetAudience(),
			int(adEntity.Priority()),
			adEntity.CreatedAt(),
			int(adEntity.MaxWaitTime().Seconds()),
			adEntity.Status(),
			shardKey,
		)
		if err != nil {
			return fmt.Errorf("failed to insert ad %s: %w", adEntity.ID().String(), err)
		}
	}

	return tx.Commit()
}

// FindByID retrieves an ad by ID from read replica
func (r *OptimizedPostgresAdRepository) FindByID(ctx context.Context, adID ad.AdID) (*ad.Ad, error) {
	db := r.getReadDB()

	// Use prepared statement if available
	if r.readStmt != nil {
		row := r.readStmt.QueryRowContext(ctx, adID.String())
		return r.scanAd(row)
	}

	// Fallback to direct query
	query := `
		SELECT ad_id, title, game_family, target_audience, priority, 
			   created_at, max_wait_time, status, processing_started_at, processed_at
		FROM ads WHERE ad_id = $1
	`

	row := db.QueryRowContext(ctx, query, adID.String())
	return r.scanAd(row)
}

// FindByIDBatch retrieves multiple ads by IDs efficiently
func (r *OptimizedPostgresAdRepository) FindByIDBatch(ctx context.Context, adIDs []ad.AdID) ([]*ad.Ad, error) {
	if len(adIDs) == 0 {
		return []*ad.Ad{}, nil
	}

	db := r.getReadDB()
	
	// Convert AdIDs to strings
	stringIDs := make([]string, len(adIDs))
	for i, id := range adIDs {
		stringIDs[i] = id.String()
	}

	// Use prepared statement if available
	var rows *sql.Rows
	var err error

	if r.batchStmt != nil {
		rows, err = r.batchStmt.QueryContext(ctx, pq.Array(stringIDs))
	} else {
		// Fallback to direct query
		query := `
			SELECT ad_id, title, game_family, target_audience, priority, 
				   created_at, max_wait_time, status, processing_started_at, processed_at
			FROM ads WHERE ad_id = ANY($1)
		`
		rows, err = db.QueryContext(ctx, query, pq.Array(stringIDs))
	}

	if err != nil {
		return nil, fmt.Errorf("failed to query ads: %w", err)
	}
	defer rows.Close()

	var ads []*ad.Ad
	for rows.Next() {
		adEntity, err := r.scanAdFromRows(rows)
		if err != nil {
			continue // Skip invalid rows
		}
		ads = append(ads, adEntity)
	}

	return ads, rows.Err()
}

// FindByStatus retrieves ads by status with pagination
func (r *OptimizedPostgresAdRepository) FindByStatus(ctx context.Context, status string, limit, offset int) ([]*ad.Ad, error) {
	db := r.getReadDB()

	query := `
		SELECT ad_id, title, game_family, target_audience, priority, 
			   created_at, max_wait_time, status, processing_started_at, processed_at
		FROM ads 
		WHERE status = $1 
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3
	`

	rows, err := db.QueryContext(ctx, query, status, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to query ads by status: %w", err)
	}
	defer rows.Close()

	var ads []*ad.Ad
	for rows.Next() {
		adEntity, err := r.scanAdFromRows(rows)
		if err != nil {
			continue
		}
		ads = append(ads, adEntity)
	}

	return ads, rows.Err()
}

// UpdateStatus updates ad status efficiently
func (r *OptimizedPostgresAdRepository) UpdateStatus(ctx context.Context, adID ad.AdID, status string) error {
	var processingStarted, processed *time.Time
	now := time.Now()

	switch status {
	case "processing":
		processingStarted = &now
	case "completed", "failed":
		processed = &now
	}

	// Use prepared statement if available
	if r.updateStmt != nil {
		_, err := r.updateStmt.ExecContext(ctx, adID.String(), status, processingStarted, processed)
		return err
	}

	// Fallback to direct query
	query := `UPDATE ads SET status = $2, processing_started_at = $3, processed_at = $4 WHERE ad_id = $1`
	_, err := r.db.ExecContext(ctx, query, adID.String(), status, processingStarted, processed)
	return err
}

// UpdateStatusBatch updates multiple ad statuses in a transaction
func (r *OptimizedPostgresAdRepository) UpdateStatusBatch(ctx context.Context, updates map[ad.AdID]string) error {
	if len(updates) == 0 {
		return nil
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx, `
		UPDATE ads SET status = $2, processing_started_at = $3, processed_at = $4 
		WHERE ad_id = $1
	`)
	if err != nil {
		return fmt.Errorf("failed to prepare update statement: %w", err)
	}
	defer stmt.Close()

	now := time.Now()
	for adID, status := range updates {
		var processingStarted, processed *time.Time

		switch status {
		case "processing":
			processingStarted = &now
		case "completed", "failed":
			processed = &now
		}

		_, err := stmt.ExecContext(ctx, adID.String(), status, processingStarted, processed)
		if err != nil {
			return fmt.Errorf("failed to update ad %s: %w", adID.String(), err)
		}
	}

	return tx.Commit()
}

// GetHealthStats returns database health statistics
func (r *OptimizedPostgresAdRepository) GetHealthStats(ctx context.Context) (*DatabaseStats, error) {
	db := r.getReadDB()

	stats := &DatabaseStats{}
	
	// Get total ads count
	err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM ads").Scan(&stats.TotalAds)
	if err != nil {
		return nil, fmt.Errorf("failed to get total ads count: %w", err)
	}

	// Get ads by status
	rows, err := db.QueryContext(ctx, "SELECT status, COUNT(*) FROM ads GROUP BY status")
	if err != nil {
		return nil, fmt.Errorf("failed to get ads by status: %w", err)
	}
	defer rows.Close()

	stats.AdsByStatus = make(map[string]int64)
	for rows.Next() {
		var status string
		var count int64
		if err := rows.Scan(&status, &count); err == nil {
			stats.AdsByStatus[status] = count
		}
	}

	// Get database connection stats
	dbStats := r.db.Stats()
	stats.OpenConnections = dbStats.OpenConnections
	stats.InUseConnections = dbStats.InUse
	stats.IdleConnections = dbStats.Idle

	return stats, nil
}

// Helper methods
func (r *OptimizedPostgresAdRepository) scanAd(row *sql.Row) (*ad.Ad, error) {
	var adID, title, gameFamily, targetAudience, status string
	var priority, maxWaitTimeSeconds int
	var createdAt time.Time
	var processingStartedAt, processedAt *time.Time

	err := row.Scan(
		&adID, &title, &gameFamily, &targetAudience, &priority,
		&createdAt, &maxWaitTimeSeconds, &status,
		&processingStartedAt, &processedAt,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to scan ad: %w", err)
	}

	return r.buildAd(adID, title, gameFamily, targetAudience, priority, createdAt, maxWaitTimeSeconds, status, processingStartedAt, processedAt)
}

func (r *OptimizedPostgresAdRepository) scanAdFromRows(rows *sql.Rows) (*ad.Ad, error) {
	var adID, title, gameFamily, targetAudience, status string
	var priority, maxWaitTimeSeconds int
	var createdAt time.Time
	var processingStartedAt, processedAt *time.Time

	err := rows.Scan(
		&adID, &title, &gameFamily, &targetAudience, &priority,
		&createdAt, &maxWaitTimeSeconds, &status,
		&processingStartedAt, &processedAt,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to scan ad from rows: %w", err)
	}

	return r.buildAd(adID, title, gameFamily, targetAudience, priority, createdAt, maxWaitTimeSeconds, status, processingStartedAt, processedAt)
}

func (r *OptimizedPostgresAdRepository) buildAd(adID, title, gameFamily, targetAudience string, priority int, createdAt time.Time, maxWaitTimeSeconds int, status string, processingStartedAt, processedAt *time.Time) (*ad.Ad, error) {
	parsedID, err := ad.ParseAdID(adID)
	if err != nil {
		return nil, fmt.Errorf("invalid ad ID: %w", err)
	}

	// Parse target audience from string to slice
	targetAudienceSlice := []string{targetAudience} // Simplified - you might want to parse JSON if multiple values
	
	factory := ad.NewFactory()
	adEntity := factory.ReconstructAd(
		parsedID,
		title,
		gameFamily,
		targetAudienceSlice,
		ad.Priority(priority),
		time.Duration(maxWaitTimeSeconds)*time.Second,
		ad.AdStatus(status),
		createdAt,
		processingStartedAt,
		processedAt,
		1, // Default version
	)

	return adEntity, nil
}

// DatabaseStats holds database performance statistics
type DatabaseStats struct {
	TotalAds          int64
	AdsByStatus       map[string]int64
	OpenConnections   int
	InUseConnections  int
	IdleConnections   int
}

// Close closes all prepared statements and connections
func (r *OptimizedPostgresAdRepository) Close() error {
	var errors []string

	if r.writeStmt != nil {
		if err := r.writeStmt.Close(); err != nil {
			errors = append(errors, fmt.Sprintf("write statement: %v", err))
		}
	}

	if r.readStmt != nil {
		if err := r.readStmt.Close(); err != nil {
			errors = append(errors, fmt.Sprintf("read statement: %v", err))
		}
	}

	if r.batchStmt != nil {
		if err := r.batchStmt.Close(); err != nil {
			errors = append(errors, fmt.Sprintf("batch statement: %v", err))
		}
	}

	if r.updateStmt != nil {
		if err := r.updateStmt.Close(); err != nil {
			errors = append(errors, fmt.Sprintf("update statement: %v", err))
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("failed to close statements: %s", strings.Join(errors, ", "))
	}

	return nil
}