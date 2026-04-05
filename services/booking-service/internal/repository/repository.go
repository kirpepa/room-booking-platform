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
	ErrBookingNotFound = errors.New("booking not found")
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
}

type BookingRepository struct {
	pool dbPool
}

func New(pool dbPool) *BookingRepository {
	return &BookingRepository{pool: pool}
}

func (r *BookingRepository) Create(ctx context.Context, b *Booking) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO bookings (id, slot_id, user_id, room_id, status, conference_requested, conference_status, slot_start_at, slot_end_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		b.ID, b.SlotID, b.UserID, b.RoomID, b.Status, b.ConferenceRequested, b.ConferenceStatus, b.SlotStartAt, b.SlotEndAt,
	)
	return err
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
		`UPDATE bookings SET status = 'cancelled', cancelled_at = NOW(), updated_at = NOW()
		 WHERE id = $1
		 RETURNING id, slot_id, user_id, room_id, status, conference_link, conference_requested, conference_status,
		           slot_start_at, slot_end_at, cancelled_at, created_at, updated_at`, id,
	).Scan(&b.ID, &b.SlotID, &b.UserID, &b.RoomID, &b.Status, &b.ConferenceLink,
		&b.ConferenceRequested, &b.ConferenceStatus,
		&b.SlotStartAt, &b.SlotEndAt, &b.CancelledAt, &b.CreatedAt, &b.UpdatedAt)
	if err != nil {
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

func (r *BookingRepository) CreateConferenceJob(ctx context.Context, bookingID uuid.UUID) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO conference_jobs (booking_id, status) VALUES ($1, 'pending') ON CONFLICT DO NOTHING`, bookingID,
	)
	return err
}

func (r *BookingRepository) GetPendingJobs(ctx context.Context, limit int) ([]ConferenceJob, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT booking_id, status, attempts, next_retry_at, last_error
		 FROM conference_jobs
		 WHERE status IN ('pending', 'processing')
		   AND (next_retry_at IS NULL OR next_retry_at <= NOW())
		 ORDER BY created_at
		 LIMIT $1
		 FOR UPDATE SKIP LOCKED`, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var jobs []ConferenceJob
	for rows.Next() {
		var j ConferenceJob
		if err := rows.Scan(&j.BookingID, &j.Status, &j.Attempts, &j.NextRetryAt, &j.LastError); err != nil {
			return nil, err
		}
		jobs = append(jobs, j)
	}
	return jobs, nil
}

func (r *BookingRepository) UpdateConferenceJob(ctx context.Context, bookingID uuid.UUID, status string, attempts int, nextRetry *time.Time, lastError *string) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE conference_jobs SET status = $2, attempts = $3, next_retry_at = $4, last_error = $5, updated_at = NOW()
		 WHERE booking_id = $1`, bookingID, status, attempts, nextRetry, lastError,
	)
	return err
}

func (r *BookingRepository) UpdateConferenceLink(ctx context.Context, bookingID uuid.UUID, link string) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE bookings SET conference_link = $2, conference_status = 'ready', updated_at = NOW()
		 WHERE id = $1`, bookingID, link,
	)
	return err
}

func (r *BookingRepository) UpdateConferenceStatus(ctx context.Context, bookingID uuid.UUID, status string) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE bookings SET conference_status = $2, updated_at = NOW()
		 WHERE id = $1`, bookingID, status,
	)
	return err
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
	return bookings, nil
}
