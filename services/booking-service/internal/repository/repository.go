package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

var (
	ErrBookingNotFound      = errors.New("booking not found")
	ErrCommitOutcomeUnknown = errors.New("transaction commit outcome is unknown")
)

// listByUserSQL implements GET /bookings/my: active bookings only, slot start in the future or now (api.yaml).
const listByUserSQL = `SELECT id, slot_id, user_id, room_id, status, conference_link, conference_requested, conference_status,
		        slot_start_at, slot_end_at, cancelled_at, created_at, updated_at
		 FROM bookings
		 WHERE user_id = $1 AND status = 'active' AND slot_start_at >= NOW()
		 ORDER BY slot_start_at`

type Booking struct {
	ID                  uuid.UUID  `json:"id"`
	SlotID              uuid.UUID  `json:"slotId"`
	UserID              uuid.UUID  `json:"userId"`
	RoomID              uuid.UUID  `json:"-"`
	Status              string     `json:"status"`
	ConferenceLink      *string    `json:"conferenceLink,omitempty"`
	ConferenceRequested bool       `json:"-"`
	ConferenceStatus    string     `json:"-"`
	SlotStartAt         time.Time  `json:"-"`
	SlotEndAt           time.Time  `json:"-"`
	CancelledAt         *time.Time `json:"-"`
	CreatedAt           time.Time  `json:"createdAt,omitempty"`
	UpdatedAt           time.Time  `json:"-"`
}

type ConferenceJob struct {
	BookingID   uuid.UUID
	Status      string
	Attempts    int
	NextRetryAt *time.Time
	LastError   *string
}

// dbPool matches *pgxpool.Pool and pgxmock pools used in tests.
type dbPool interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
	Begin(ctx context.Context) (pgx.Tx, error)
}

type BookingRepository struct {
	pool dbPool
}

func New(pool dbPool) *BookingRepository {
	return &BookingRepository{pool: pool}
}

func (r *BookingRepository) CreateWithConferenceJob(ctx context.Context, b *Booking) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin create booking transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	err = tx.QueryRow(ctx,
		`INSERT INTO bookings (id, slot_id, user_id, room_id, status, conference_requested, conference_status, slot_start_at, slot_end_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		 RETURNING created_at, updated_at`,
		b.ID, b.SlotID, b.UserID, b.RoomID, b.Status, b.ConferenceRequested, b.ConferenceStatus, b.SlotStartAt, b.SlotEndAt,
	).Scan(&b.CreatedAt, &b.UpdatedAt)
	if err != nil {
		return fmt.Errorf("insert booking: %w", err)
	}

	if b.ConferenceRequested {
		if _, err := tx.Exec(ctx,
			`INSERT INTO conference_jobs (booking_id, status) VALUES ($1, 'pending')`, b.ID,
		); err != nil {
			return fmt.Errorf("insert conference job: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		if !errors.Is(err, pgx.ErrTxCommitRollback) {
			return fmt.Errorf("commit create booking transaction: %w", errors.Join(ErrCommitOutcomeUnknown, err))
		}
		return fmt.Errorf("commit create booking transaction: %w", err)
	}
	return nil
}

func (r *BookingRepository) GetByID(ctx context.Context, id uuid.UUID) (*Booking, error) {
	var b Booking
	err := r.pool.QueryRow(ctx,
		`SELECT id, slot_id, user_id, room_id, status, conference_link, conference_requested, conference_status,
		        slot_start_at, slot_end_at, cancelled_at, created_at, updated_at
		 FROM bookings WHERE id = $1`, id,
	).Scan(&b.ID, &b.SlotID, &b.UserID, &b.RoomID, &b.Status, &b.ConferenceLink,
		&b.ConferenceRequested, &b.ConferenceStatus,
		&b.SlotStartAt, &b.SlotEndAt, &b.CancelledAt, &b.CreatedAt, &b.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrBookingNotFound
		}
		return nil, fmt.Errorf("get booking: %w", err)
	}
	return &b, nil
}

func (r *BookingRepository) Cancel(ctx context.Context, id uuid.UUID) (*Booking, error) {
	var b Booking
	err := r.pool.QueryRow(ctx,
		`UPDATE bookings
		 SET status = 'cancelled', cancelled_at = NOW(),
		     conference_status = CASE WHEN conference_requested THEN 'not_requested' ELSE conference_status END,
		     conference_link = NULL,
		     updated_at = NOW()
		 WHERE id = $1
		 RETURNING id, slot_id, user_id, room_id, status, conference_link, conference_requested, conference_status,
		           slot_start_at, slot_end_at, cancelled_at, created_at, updated_at`, id,
	).Scan(&b.ID, &b.SlotID, &b.UserID, &b.RoomID, &b.Status, &b.ConferenceLink,
		&b.ConferenceRequested, &b.ConferenceStatus,
		&b.SlotStartAt, &b.SlotEndAt, &b.CancelledAt, &b.CreatedAt, &b.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrBookingNotFound
		}
		return nil, fmt.Errorf("cancel booking: %w", err)
	}
	return &b, nil
}

func (r *BookingRepository) ListByUser(ctx context.Context, userID uuid.UUID) ([]Booking, error) {
	rows, err := r.pool.Query(ctx, listByUserSQL, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanBookings(rows)
}

func (r *BookingRepository) ListAll(ctx context.Context, page, pageSize int) ([]Booking, int, error) {
	var total int
	err := r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM bookings`).Scan(&total)
	if err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * pageSize
	rows, err := r.pool.Query(ctx,
		`SELECT id, slot_id, user_id, room_id, status, conference_link, conference_requested, conference_status,
		        slot_start_at, slot_end_at, cancelled_at, created_at, updated_at
		 FROM bookings ORDER BY created_at DESC LIMIT $1 OFFSET $2`, pageSize, offset,
	)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	bookings, err := scanBookings(rows)
	return bookings, total, err
}

func (r *BookingRepository) ClaimConferenceJobs(ctx context.Context, limit int, lease time.Duration) ([]ConferenceJob, error) {
	if limit <= 0 {
		return []ConferenceJob{}, nil
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin claim jobs transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	leaseMicros := lease.Microseconds()
	rows, err := tx.Query(ctx,
		`WITH candidates AS (
			SELECT booking_id
			FROM conference_jobs
			WHERE (status = 'pending' AND (next_retry_at IS NULL OR next_retry_at <= NOW()))
			   OR (status = 'processing' AND (locked_at IS NULL OR locked_at <= NOW() - ($2::bigint * INTERVAL '1 microsecond')))
			ORDER BY created_at
			LIMIT $1
			FOR UPDATE SKIP LOCKED
		)
		UPDATE conference_jobs AS jobs
		SET status = 'processing', locked_at = NOW(), updated_at = NOW()
		FROM candidates
		WHERE jobs.booking_id = candidates.booking_id
		RETURNING jobs.booking_id, jobs.status, jobs.attempts, jobs.next_retry_at, jobs.last_error`,
		limit, leaseMicros,
	)
	if err != nil {
		return nil, fmt.Errorf("claim conference jobs: %w", err)
	}

	var jobs []ConferenceJob
	for rows.Next() {
		var j ConferenceJob
		if err := rows.Scan(&j.BookingID, &j.Status, &j.Attempts, &j.NextRetryAt, &j.LastError); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan claimed conference job: %w", err)
		}
		jobs = append(jobs, j)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, fmt.Errorf("iterate claimed conference jobs: %w", err)
	}
	rows.Close()

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit claimed conference jobs: %w", err)
	}
	if jobs == nil {
		jobs = []ConferenceJob{}
	}
	return jobs, nil
}

func (r *BookingRepository) RescheduleConferenceJob(ctx context.Context, bookingID uuid.UUID, attempts int, nextRetry time.Time, lastError string) error {
	tag, err := r.pool.Exec(ctx,
		`UPDATE conference_jobs
		 SET status = 'pending', attempts = $2, next_retry_at = $3, last_error = $4,
		     locked_at = NULL, updated_at = NOW()
		 WHERE booking_id = $1 AND status = 'processing'`,
		bookingID, attempts, nextRetry, lastError,
	)
	return expectOne(tag, err, "reschedule conference job")
}

func (r *BookingRepository) CompleteConferenceJob(ctx context.Context, bookingID uuid.UUID, link string, attempts int) error {
	return r.finishConferenceJob(ctx, bookingID,
		`UPDATE bookings
		 SET conference_link = $2, conference_status = 'ready', updated_at = NOW()
		 WHERE id = $1 AND status = 'active'`,
		[]any{bookingID, link}, "done", attempts, nil)
}

func (r *BookingRepository) FailConferenceJob(ctx context.Context, bookingID uuid.UUID, attempts int, lastError string) error {
	return r.finishConferenceJob(ctx, bookingID,
		`UPDATE bookings
		 SET conference_status = 'failed', updated_at = NOW()
		 WHERE id = $1 AND status = 'active'`,
		[]any{bookingID}, "failed", attempts, &lastError)
}

func (r *BookingRepository) DiscardConferenceJob(ctx context.Context, bookingID uuid.UUID, attempts int) error {
	return r.finishConferenceJob(ctx, bookingID,
		`UPDATE bookings
		 SET conference_status = 'not_requested', updated_at = NOW()
		 WHERE id = $1`,
		[]any{bookingID}, "done", attempts, nil)
}

func (r *BookingRepository) finishConferenceJob(
	ctx context.Context,
	bookingID uuid.UUID,
	bookingSQL string,
	bookingArgs []any,
	jobStatus string,
	attempts int,
	lastError *string,
) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin finish conference job transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, bookingSQL, bookingArgs...); err != nil {
		return fmt.Errorf("update booking conference state: %w", err)
	}
	tag, err := tx.Exec(ctx,
		`UPDATE conference_jobs
		 SET status = $2, attempts = $3, next_retry_at = NULL, last_error = $4,
		     locked_at = NULL, updated_at = NOW()
		 WHERE booking_id = $1 AND status = 'processing'`,
		bookingID, jobStatus, attempts, lastError,
	)
	if err := expectOne(tag, err, "finish conference job"); err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit finish conference job transaction: %w", err)
	}
	return nil
}

func expectOne(tag pgconn.CommandTag, err error, operation string) error {
	if err != nil {
		return fmt.Errorf("%s: %w", operation, err)
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("%s: expected one affected row, got %d", operation, tag.RowsAffected())
	}
	return nil
}

func scanBookings(rows pgx.Rows) ([]Booking, error) {
	var bookings []Booking
	for rows.Next() {
		var b Booking
		if err := rows.Scan(&b.ID, &b.SlotID, &b.UserID, &b.RoomID, &b.Status, &b.ConferenceLink,
			&b.ConferenceRequested, &b.ConferenceStatus,
			&b.SlotStartAt, &b.SlotEndAt, &b.CancelledAt, &b.CreatedAt, &b.UpdatedAt); err != nil {
			return nil, err
		}
		bookings = append(bookings, b)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate bookings: %w", err)
	}
	return bookings, nil
}
