package persistence

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/lib/pq"
	_ "github.com/lib/pq"

	"github.com/personal/home-work-ad-process/internal/domain/ad"
)

// PostgresAdRepository implements the ad.Repository interface using PostgreSQL
type PostgresAdRepository struct {
	db *sql.DB
}

// NewPostgresAdRepository creates a new PostgresAdRepository
func NewPostgresAdRepository(db *sql.DB) *PostgresAdRepository {
	return &PostgresAdRepository{db: db}
}

// adRow represents an ad row in the database
type adRow struct {
	AdID                string         `db:"ad_id"`
	Title               string         `db:"title"`
	GameFamily          string         `db:"game_family"`
	TargetAudience      []byte         `db:"target_audience"`
	Priority            int            `db:"priority"`
	CreatedAt           time.Time      `db:"created_at"`
	MaxWaitTime         int            `db:"max_wait_time"`
	Status              string         `db:"status"`
	ProcessingStartedAt *time.Time     `db:"processing_started_at"`
	ProcessedAt         *time.Time     `db:"processed_at"`
	Version             int            `db:"version"`
}

// toAdEntity converts a database row to an ad entity
func (r *adRow) toAdEntity() (*ad.Ad, error) {
	adID, err := ad.ParseAdID(r.AdID)
	if err != nil {
		return nil, err
	}
	
	var targetAudience []string
	if err := json.Unmarshal(r.TargetAudience, &targetAudience); err != nil {
		return nil, fmt.Errorf("failed to unmarshal target audience: %w", err)
	}
	
	maxWaitTime := time.Duration(r.MaxWaitTime) * time.Second
	
	// Use factory to reconstruct the entity from persistence
	factory := ad.NewFactory()
	adEntity := factory.ReconstructAd(
		adID,
		r.Title,
		r.GameFamily,
		targetAudience,
		ad.Priority(r.Priority),
		maxWaitTime,
		ad.AdStatus(r.Status),
		r.CreatedAt,
		r.ProcessingStartedAt,
		r.ProcessedAt,
		r.Version,
	)
	
	return adEntity, nil
}

// Save saves an ad to the database
func (r *PostgresAdRepository) Save(ctx context.Context, adEntity *ad.Ad) error {
	targetAudienceJSON, err := json.Marshal(adEntity.TargetAudience())
	if err != nil {
		return fmt.Errorf("failed to marshal target audience: %w", err)
	}
	
	query := `
		INSERT INTO ads (ad_id, title, game_family, target_audience, priority, created_at, max_wait_time, status, version)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT (ad_id) DO UPDATE SET
			title = EXCLUDED.title,
			game_family = EXCLUDED.game_family,
			target_audience = EXCLUDED.target_audience,
			priority = EXCLUDED.priority,
			max_wait_time = EXCLUDED.max_wait_time,
			status = EXCLUDED.status,
			processing_started_at = EXCLUDED.processing_started_at,
			processed_at = EXCLUDED.processed_at,
			version = EXCLUDED.version
		WHERE ads.version = $9 - 1
	`
	
	result, err := r.db.ExecContext(ctx, query,
		adEntity.ID().String(),
		adEntity.Title(),
		adEntity.GameFamily(),
		targetAudienceJSON,
		int(adEntity.Priority()),
		adEntity.CreatedAt(),
		int(adEntity.MaxWaitTime().Seconds()),
		string(adEntity.Status()),
		adEntity.Version(),
	)
	
	if err != nil {
		if pqErr, ok := err.(*pq.Error); ok && pqErr.Code == "23505" {
			return ad.ErrOptimisticLockFailed
		}
		return fmt.Errorf("failed to save ad: %w", err)
	}
	
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	
	if rowsAffected == 0 {
		return ad.ErrOptimisticLockFailed
	}
	
	return nil
}

// FindByID finds an ad by its ID
func (r *PostgresAdRepository) FindByID(ctx context.Context, id ad.AdID) (*ad.Ad, error) {
	query := `
		SELECT ad_id, title, game_family, target_audience, priority, created_at, 
			   max_wait_time, status, processing_started_at, processed_at, version
		FROM ads 
		WHERE ad_id = $1
	`
	
	var row adRow
	err := r.db.QueryRowContext(ctx, query, id.String()).Scan(
		&row.AdID,
		&row.Title,
		&row.GameFamily,
		&row.TargetAudience,
		&row.Priority,
		&row.CreatedAt,
		&row.MaxWaitTime,
		&row.Status,
		&row.ProcessingStartedAt,
		&row.ProcessedAt,
		&row.Version,
	)
	
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ad.ErrAdNotFound
		}
		return nil, fmt.Errorf("failed to find ad by ID: %w", err)
	}
	
	return row.toAdEntity()
}

// FindByGameFamily finds all ads for a specific game family
func (r *PostgresAdRepository) FindByGameFamily(ctx context.Context, gameFamily string) ([]*ad.Ad, error) {
	query := `
		SELECT ad_id, title, game_family, target_audience, priority, created_at, 
			   max_wait_time, status, processing_started_at, processed_at, version
		FROM ads 
		WHERE game_family = $1
		ORDER BY created_at ASC
	`
	
	rows, err := r.db.QueryContext(ctx, query, gameFamily)
	if err != nil {
		return nil, fmt.Errorf("failed to find ads by game family: %w", err)
	}
	defer rows.Close()
	
	var ads []*ad.Ad
	for rows.Next() {
		var row adRow
		err := rows.Scan(
			&row.AdID,
			&row.Title,
			&row.GameFamily,
			&row.TargetAudience,
			&row.Priority,
			&row.CreatedAt,
			&row.MaxWaitTime,
			&row.Status,
			&row.ProcessingStartedAt,
			&row.ProcessedAt,
			&row.Version,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan ad row: %w", err)
		}
		
		adEntity, err := row.toAdEntity()
		if err != nil {
			return nil, fmt.Errorf("failed to convert row to entity: %w", err)
		}
		
		ads = append(ads, adEntity)
	}
	
	return ads, nil
}

// FindByStatus finds all ads with a specific status
func (r *PostgresAdRepository) FindByStatus(ctx context.Context, status ad.AdStatus) ([]*ad.Ad, error) {
	query := `
		SELECT ad_id, title, game_family, target_audience, priority, created_at, 
			   max_wait_time, status, processing_started_at, processed_at, version
		FROM ads 
		WHERE status = $1
		ORDER BY created_at ASC
	`
	
	rows, err := r.db.QueryContext(ctx, query, string(status))
	if err != nil {
		return nil, fmt.Errorf("failed to find ads by status: %w", err)
	}
	defer rows.Close()
	
	var ads []*ad.Ad
	for rows.Next() {
		var row adRow
		err := rows.Scan(
			&row.AdID,
			&row.Title,
			&row.GameFamily,
			&row.TargetAudience,
			&row.Priority,
			&row.CreatedAt,
			&row.MaxWaitTime,
			&row.Status,
			&row.ProcessingStartedAt,
			&row.ProcessedAt,
			&row.Version,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan ad row: %w", err)
		}
		
		adEntity, err := row.toAdEntity()
		if err != nil {
			return nil, fmt.Errorf("failed to convert row to entity: %w", err)
		}
		
		ads = append(ads, adEntity)
	}
	
	return ads, nil
}

// FindOlderThan finds all ads created before the specified time
func (r *PostgresAdRepository) FindOlderThan(ctx context.Context, threshold time.Time) ([]*ad.Ad, error) {
	query := `
		SELECT ad_id, title, game_family, target_audience, priority, created_at, 
			   max_wait_time, status, processing_started_at, processed_at, version
		FROM ads 
		WHERE created_at < $1
		ORDER BY created_at ASC
	`
	
	rows, err := r.db.QueryContext(ctx, query, threshold)
	if err != nil {
		return nil, fmt.Errorf("failed to find old ads: %w", err)
	}
	defer rows.Close()
	
	var ads []*ad.Ad
	for rows.Next() {
		var row adRow
		err := rows.Scan(
			&row.AdID,
			&row.Title,
			&row.GameFamily,
			&row.TargetAudience,
			&row.Priority,
			&row.CreatedAt,
			&row.MaxWaitTime,
			&row.Status,
			&row.ProcessingStartedAt,
			&row.ProcessedAt,
			&row.Version,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan ad row: %w", err)
		}
		
		adEntity, err := row.toAdEntity()
		if err != nil {
			return nil, fmt.Errorf("failed to convert row to entity: %w", err)
		}
		
		ads = append(ads, adEntity)
	}
	
	return ads, nil
}

// UpdatePriorityBatch updates priority for multiple ads atomically
func (r *PostgresAdRepository) UpdatePriorityBatch(ctx context.Context, ids []ad.AdID, newPriority ad.Priority) error {
	if len(ids) == 0 {
		return nil
	}
	
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()
	
	query := `UPDATE ads SET priority = $1, version = version + 1 WHERE ad_id = $2 AND status = 'queued'`
	
	var updatedCount int
	for _, id := range ids {
		result, err := tx.ExecContext(ctx, query, int(newPriority), id.String())
		if err != nil {
			return fmt.Errorf("failed to update ad priority: %w", err)
		}
		
		rowsAffected, err := result.RowsAffected()
		if err != nil {
			return fmt.Errorf("failed to get rows affected: %w", err)
		}
		
		updatedCount += int(rowsAffected)
	}
	
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}
	
	return nil
}

// UpdateStatus updates the status of an ad
func (r *PostgresAdRepository) UpdateStatus(ctx context.Context, id ad.AdID, status ad.AdStatus, version int) error {
	query := `
		UPDATE ads 
		SET status = $1, version = version + 1
		WHERE ad_id = $2 AND version = $3
	`
	
	result, err := r.db.ExecContext(ctx, query, string(status), id.String(), version)
	if err != nil {
		return fmt.Errorf("failed to update ad status: %w", err)
	}
	
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	
	if rowsAffected == 0 {
		return ad.ErrOptimisticLockFailed
	}
	
	return nil
}

// Delete removes an ad from the repository
func (r *PostgresAdRepository) Delete(ctx context.Context, id ad.AdID) error {
	query := `DELETE FROM ads WHERE ad_id = $1`
	
	result, err := r.db.ExecContext(ctx, query, id.String())
	if err != nil {
		return fmt.Errorf("failed to delete ad: %w", err)
	}
	
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	
	if rowsAffected == 0 {
		return ad.ErrAdNotFound
	}
	
	return nil
}

// Count returns the total number of ads
func (r *PostgresAdRepository) Count(ctx context.Context) (int64, error) {
	query := `SELECT COUNT(*) FROM ads`
	
	var count int64
	err := r.db.QueryRowContext(ctx, query).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count ads: %w", err)
	}
	
	return count, nil
}

// CountByStatus returns the number of ads with a specific status
func (r *PostgresAdRepository) CountByStatus(ctx context.Context, status ad.AdStatus) (int64, error) {
	query := `SELECT COUNT(*) FROM ads WHERE status = $1`
	
	var count int64
	err := r.db.QueryRowContext(ctx, query, string(status)).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count ads by status: %w", err)
	}
	
	return count, nil
}

// CountByPriority returns the number of ads with a specific priority
func (r *PostgresAdRepository) CountByPriority(ctx context.Context, priority ad.Priority) (int64, error) {
	query := `SELECT COUNT(*) FROM ads WHERE priority = $1`
	
	var count int64
	err := r.db.QueryRowContext(ctx, query, int(priority)).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count ads by priority: %w", err)
	}
	
	return count, nil
}