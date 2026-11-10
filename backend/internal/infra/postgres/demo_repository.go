package postgres

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// DemoRepository cleans up after the public demo's shared visitor account.
type DemoRepository struct {
	pool *pgxpool.Pool
}

// NewDemoRepository builds the repository.
func NewDemoRepository(pool *pgxpool.Pool) *DemoRepository {
	return &DemoRepository{pool: pool}
}

// CleanupDemo deletes the account's query history, saved queries and table
// views created before the cutoff, and returns how many rows went.
func (r *DemoRepository) CleanupDemo(ctx context.Context, userID uuid.UUID, before time.Time) (int64, error) {
	var total int64
	for _, q := range []string{
		`DELETE FROM query_history WHERE user_id = $1 AND created_at < $2`,
		`DELETE FROM saved_queries WHERE user_id = $1 AND created_at < $2`,
		`DELETE FROM table_views WHERE created_by = $1 AND created_at < $2`,
	} {
		tag, err := r.pool.Exec(ctx, q, userID, before)
		if err != nil {
			return total, err
		}
		total += tag.RowsAffected()
	}
	return total, nil
}
