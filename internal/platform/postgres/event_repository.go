package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/woyaofei303/exchange-lab/internal/eventing"
)

// EventRepository owns Outbox leases, receipts, deduplication and parking.
type EventRepository struct {
	pool    *pgxpool.Pool
	timeout time.Duration
}

// NewEventRepository constructs a context-bounded durable event repository.
func NewEventRepository(pool *pgxpool.Pool, timeout time.Duration) *EventRepository {
	return &EventRepository{pool: pool, timeout: timeout}
}

// Claim leases a bounded ordered batch using SKIP LOCKED for concurrent publishers.
func (r *EventRepository) Claim(ctx context.Context, limit int, lease time.Duration) ([]eventing.OutboxEvent, error) {
	if r == nil || r.pool == nil || r.timeout <= 0 || limit < 1 || limit > 1000 || lease <= 0 {
		return nil, fmt.Errorf("claim outbox: invalid configuration")
	}
	operation, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()
	leaseUntil := time.Now().UTC().Add(lease)
	rows, err := r.pool.Query(operation, `
		WITH candidates AS (
			SELECT id FROM outbox_events
			WHERE status IN ('pending','publishing') AND available_at <= now()
			ORDER BY available_at,created_at,id
			FOR UPDATE SKIP LOCKED LIMIT $1
		)
		UPDATE outbox_events AS events
		SET status='publishing',attempts=events.attempts+1,available_at=$2
		FROM candidates WHERE events.id=candidates.id
		RETURNING events.id,events.aggregate_type,events.aggregate_id,events.subject,events.event_version,
		          events.correlation_id,events.payload,events.attempts,events.created_at
	`, limit, leaseUntil)
	if err != nil {
		return nil, fmt.Errorf("claim outbox: %w", err)
	}
	defer rows.Close()
	result := make([]eventing.OutboxEvent, 0, limit)
	for rows.Next() {
		var event eventing.OutboxEvent
		if err := rows.Scan(&event.ID, &event.AggregateType, &event.AggregateID, &event.Subject, &event.Version, &event.CorrelationID, &event.Payload, &event.Attempts, &event.CreatedAt); err != nil {
			return nil, fmt.Errorf("claim outbox: scan: %w", err)
		}
		result = append(result, event)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("claim outbox: rows: %w", err)
	}
	return result, nil
}

// MarkPublished completes exactly one currently leased Outbox event.
func (r *EventRepository) MarkPublished(ctx context.Context, id string, publishedAt time.Time) error {
	operation, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()
	result, err := r.pool.Exec(operation, `UPDATE outbox_events SET status='published',published_at=$2,last_error_code=NULL WHERE id=$1 AND status='publishing'`, id, publishedAt.UTC())
	if err != nil {
		return fmt.Errorf("mark outbox published: %w", err)
	}
	if result.RowsAffected() != 1 {
		return fmt.Errorf("mark outbox published: event not claimed")
	}
	return nil
}

// MarkFailed schedules bounded retry or parks an exhausted Outbox event.
func (r *EventRepository) MarkFailed(ctx context.Context, event eventing.OutboxEvent, reasonCode string, failedAt time.Time) error {
	if reasonCode == "" || len(reasonCode) > 64 {
		return fmt.Errorf("mark outbox failed: invalid reason")
	}
	status := "pending"
	nextAttempt := failedAt.UTC().Add(retryDelay(event.Attempts))
	if event.Attempts >= 5 {
		status = "parked"
		nextAttempt = failedAt.UTC()
	}
	operation, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()
	result, err := r.pool.Exec(operation, `UPDATE outbox_events SET status=$2,available_at=$3,last_error_code=$4 WHERE id=$1 AND status='publishing'`, event.ID, status, nextAttempt, reasonCode)
	if err != nil {
		return fmt.Errorf("mark outbox failed: %w", err)
	}
	if result.RowsAffected() != 1 {
		return fmt.Errorf("mark outbox failed: event not claimed")
	}
	return nil
}

// RecordReceipt atomically deduplicates an event and appends its receipt side effect.
func (r *EventRepository) RecordReceipt(ctx context.Context, consumer string, envelope eventing.Envelope, receivedAt time.Time) (bool, error) {
	if consumer == "" || len(consumer) > 128 || envelope.ID == "" {
		return false, fmt.Errorf("record event receipt: invalid input")
	}
	operation, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()
	tx, err := r.pool.BeginTx(operation, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return false, fmt.Errorf("record event receipt: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	result, err := tx.Exec(operation, `INSERT INTO processed_events(consumer_name,event_id,subject,processed_at) VALUES ($1,$2,$3,$4) ON CONFLICT DO NOTHING`, consumer, envelope.ID, envelope.Type, receivedAt.UTC())
	if err != nil {
		return false, fmt.Errorf("record event receipt: processed event: %w", err)
	}
	if result.RowsAffected() == 0 {
		if err := tx.Commit(operation); err != nil {
			return false, fmt.Errorf("record event receipt replay: commit: %w", err)
		}
		return false, nil
	}
	if _, err := tx.Exec(operation, `INSERT INTO event_receipts(consumer_name,event_id,aggregate_id,correlation_id,received_at) VALUES ($1,$2,$3,$4,$5)`, consumer, envelope.ID, envelope.AggregateID, envelope.CorrelationID, receivedAt.UTC()); err != nil {
		return false, fmt.Errorf("record event receipt: side effect: %w", err)
	}
	if err := tx.Commit(operation); err != nil {
		return false, fmt.Errorf("record event receipt: commit: %w", err)
	}
	return true, nil
}

// Park records a terminal consumer failure without deleting the original payload.
func (r *EventRepository) Park(ctx context.Context, consumer string, envelope eventing.Envelope, payload []byte, deliveryCount uint64, reasonCode string, parkedAt time.Time) error {
	if consumer == "" || envelope.ID == "" || deliveryCount == 0 || reasonCode == "" {
		return fmt.Errorf("park event: invalid input")
	}
	operation, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()
	_, err := r.pool.Exec(operation, `
		INSERT INTO parked_events(consumer_name,event_id,subject,reason_code,delivery_count,payload,parked_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7)
		ON CONFLICT (consumer_name,event_id) DO UPDATE SET
			reason_code=EXCLUDED.reason_code,delivery_count=GREATEST(parked_events.delivery_count,EXCLUDED.delivery_count),parked_at=EXCLUDED.parked_at
	`, consumer, envelope.ID, envelope.Type, reasonCode, deliveryCount, payload, parkedAt.UTC())
	if err != nil {
		return fmt.Errorf("park event: %w", err)
	}
	return nil
}

func retryDelay(attempt int) time.Duration {
	switch {
	case attempt <= 1:
		return time.Second
	case attempt == 2:
		return 2 * time.Second
	case attempt == 3:
		return 5 * time.Second
	default:
		return 30 * time.Second
	}
}

// IsSerializationFailure reports PostgreSQL SQLSTATE 40001 for bounded retry.
func IsSerializationFailure(err error) bool {
	var postgresError interface{ SQLState() string }
	return errors.As(err, &postgresError) && postgresError.SQLState() == "40001"
}
