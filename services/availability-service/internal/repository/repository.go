package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrScheduleExists  = errors.New("schedule already exists")
	ErrScheduleNotFound = errors.New("schedule not found")
	ErrSlotNotFound    = errors.New("slot not found")
	ErrSlotBooked      = errors.New("slot already booked")
	ErrSlotInPast      = errors.New("slot is in the past")
)

type Schedule struct {
	ID                  uuid.UUID `json:"id"`
	RoomID              uuid.UUID `json:"roomId"`
	DaysOfWeek          []int16   `json:"daysOfWeek"`
	StartTime           string    `json:"startTime"`
	EndTime             string    `json:"endTime"`
	SlotDurationMinutes int16     `json:"-"`
	CreatedAt           time.Time `json:"-"`
}

type Slot struct {
	ID               uuid.UUID  `json:"id"`
	RoomID           uuid.UUID  `json:"roomId"`
	ScheduleID       uuid.UUID  `json:"-"`
	StartAt          time.Time  `json:"start"`
	EndAt            time.Time  `json:"end"`
	Status           string     `json:"-"`
	CurrentBookingID *uuid.UUID `json:"-"`
	CreatedAt        time.Time  `json:"-"`
}

type SlotMeta struct {
	SlotID  uuid.UUID `json:"slot_id"`
	RoomID  uuid.UUID `json:"room_id"`
	StartAt time.Time `json:"start_at"`
	EndAt   time.Time `json:"end_at"`
	Status  string    `json:"status"`
}

type Repository struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

func (r *Repository) CreateSchedule(ctx context.Context, id, roomID uuid.UUID, daysOfWeek []int16, startTime, endTime string) (*Schedule, error) {
	var s Schedule
	err := r.pool.QueryRow(ctx,
		`INSERT INTO schedules (id, room_id, days_of_week, start_time, end_time)
		 VALUES ($1, $2, $3, $4::time, $5::time)
		 RETURNING id, room_id, days_of_week, start_time::text, end_time::text, slot_duration_minutes, created_at`,
		id, roomID, daysOfWeek, startTime, endTime,
	).Scan(&s.ID, &s.RoomID, &s.DaysOfWeek, &s.StartTime, &s.EndTime, &s.SlotDurationMinutes, &s.CreatedAt)
	if err != nil {
		errStr := err.Error()
		if containsStr(errStr, "duplicate key") || containsStr(errStr, "23505") {
			return nil, ErrScheduleExists
		}
		return nil, fmt.Errorf("insert schedule: %w", err)
	}
	// Trim time strings to HH:MM
	if len(s.StartTime) > 5 {
		s.StartTime = s.StartTime[:5]
	}
	if len(s.EndTime) > 5 {
		s.EndTime = s.EndTime[:5]
	}
	return &s, nil
}

func (r *Repository) GetScheduleByRoomID(ctx context.Context, roomID uuid.UUID) (*Schedule, error) {
	var s Schedule
	err := r.pool.QueryRow(ctx,
		`SELECT id, room_id, days_of_week, start_time::text, end_time::text, slot_duration_minutes, created_at
		 FROM schedules WHERE room_id = $1`, roomID,
	).Scan(&s.ID, &s.RoomID, &s.DaysOfWeek, &s.StartTime, &s.EndTime, &s.SlotDurationMinutes, &s.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrScheduleNotFound
		}
		return nil, fmt.Errorf("get schedule: %w", err)
	}
	if len(s.StartTime) > 5 {
		s.StartTime = s.StartTime[:5]
	}
	if len(s.EndTime) > 5 {
		s.EndTime = s.EndTime[:5]
	}
	return &s, nil
}

func (r *Repository) GetFreeSlots(ctx context.Context, roomID uuid.UUID, date time.Time) ([]Slot, error) {
	dayStart := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, time.UTC)
	dayEnd := dayStart.Add(24 * time.Hour)

	rows, err := r.pool.Query(ctx,
		`SELECT id, room_id, schedule_id, start_at, end_at, status, current_booking_id, created_at
		 FROM slots
		 WHERE room_id = $1 AND start_at >= $2 AND start_at < $3 AND status = 'free'
		 ORDER BY start_at`,
		roomID, dayStart, dayEnd,
	)
	if err != nil {
		return nil, fmt.Errorf("query free slots: %w", err)
	}
	defer rows.Close()

	var slots []Slot
	for rows.Next() {
		var s Slot
		if err := rows.Scan(&s.ID, &s.RoomID, &s.ScheduleID, &s.StartAt, &s.EndAt, &s.Status, &s.CurrentBookingID, &s.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan slot: %w", err)
		}
		slots = append(slots, s)
	}
	return slots, nil
}

func (r *Repository) GetSlotByID(ctx context.Context, slotID uuid.UUID) (*Slot, error) {
	var s Slot
	err := r.pool.QueryRow(ctx,
		`SELECT id, room_id, schedule_id, start_at, end_at, status, current_booking_id, created_at
		 FROM slots WHERE id = $1`, slotID,
	).Scan(&s.ID, &s.RoomID, &s.ScheduleID, &s.StartAt, &s.EndAt, &s.Status, &s.CurrentBookingID, &s.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrSlotNotFound
		}
		return nil, fmt.Errorf("get slot: %w", err)
	}
	return &s, nil
}

func (r *Repository) BookSlot(ctx context.Context, slotID, bookingID uuid.UUID) (*SlotMeta, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	var s SlotMeta
	var status string
	var startAt time.Time

	err = tx.QueryRow(ctx,
		`SELECT id, room_id, start_at, end_at, status FROM slots WHERE id = $1 FOR UPDATE`, slotID,
	).Scan(&s.SlotID, &s.RoomID, &startAt, &s.EndAt, &status)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrSlotNotFound
		}
		return nil, fmt.Errorf("select slot for update: %w", err)
	}
	s.StartAt = startAt

	if startAt.Before(time.Now().UTC()) {
		return nil, ErrSlotInPast
	}
	if status != "free" {
		return nil, ErrSlotBooked
	}

	_, err = tx.Exec(ctx,
		`UPDATE slots SET status = 'booked', current_booking_id = $1 WHERE id = $2`,
		bookingID, slotID,
	)
	if err != nil {
		return nil, fmt.Errorf("update slot: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}

	s.Status = "booked"
	return &s, nil
}

func (r *Repository) ReleaseSlot(ctx context.Context, slotID, bookingID uuid.UUID) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE slots SET status = 'free', current_booking_id = NULL
		 WHERE id = $1 AND current_booking_id = $2 AND status = 'booked'`,
		slotID, bookingID,
	)
	// Idempotent: if no rows affected, slot was already free or booking doesn't match
	return err
}

func (r *Repository) GenerateSlots(ctx context.Context, schedule *Schedule, from, until time.Time) (int, error) {
	startTime, err := time.Parse("15:04", schedule.StartTime)
	if err != nil {
		return 0, fmt.Errorf("parse start time: %w", err)
	}
	endTime, err := time.Parse("15:04", schedule.EndTime)
	if err != nil {
		return 0, fmt.Errorf("parse end time: %w", err)
	}

	daysSet := make(map[time.Weekday]bool)
	for _, d := range schedule.DaysOfWeek {
		// 1=Mon..7=Sun -> Go: Mon=1..Sun=0
		wd := time.Weekday(d % 7)
		daysSet[wd] = true
	}

	count := 0
	for date := from; date.Before(until); date = date.AddDate(0, 0, 1) {
		if !daysSet[date.Weekday()] {
			continue
		}

		slotStart := time.Date(date.Year(), date.Month(), date.Day(),
			startTime.Hour(), startTime.Minute(), 0, 0, time.UTC)
		dayEnd := time.Date(date.Year(), date.Month(), date.Day(),
			endTime.Hour(), endTime.Minute(), 0, 0, time.UTC)

		for slotStart.Before(dayEnd) {
			slotEnd := slotStart.Add(30 * time.Minute)
			id := uuid.New()
			_, err := r.pool.Exec(ctx,
				`INSERT INTO slots (id, room_id, schedule_id, start_at, end_at)
				 VALUES ($1, $2, $3, $4, $5)
				 ON CONFLICT (room_id, start_at) DO NOTHING`,
				id, schedule.RoomID, schedule.ID, slotStart, slotEnd,
			)
			if err != nil {
				return count, fmt.Errorf("insert slot: %w", err)
			}
			count++
			slotStart = slotEnd
		}
	}
	return count, nil
}

func (r *Repository) GetGenerationState(ctx context.Context, roomID uuid.UUID) (*time.Time, error) {
	var until time.Time
	err := r.pool.QueryRow(ctx,
		`SELECT generated_until FROM slot_generation_state WHERE room_id = $1`, roomID,
	).Scan(&until)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &until, nil
}

func (r *Repository) UpsertGenerationState(ctx context.Context, roomID uuid.UUID, until time.Time) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO slot_generation_state (room_id, generated_until, updated_at)
		 VALUES ($1, $2, NOW())
		 ON CONFLICT (room_id) DO UPDATE SET generated_until = $2, updated_at = NOW()`,
		roomID, until,
	)
	return err
}

func (r *Repository) GetAllSchedules(ctx context.Context) ([]Schedule, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, room_id, days_of_week, start_time::text, end_time::text, slot_duration_minutes, created_at FROM schedules`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var schedules []Schedule
	for rows.Next() {
		var s Schedule
		if err := rows.Scan(&s.ID, &s.RoomID, &s.DaysOfWeek, &s.StartTime, &s.EndTime, &s.SlotDurationMinutes, &s.CreatedAt); err != nil {
			return nil, err
		}
		if len(s.StartTime) > 5 {
			s.StartTime = s.StartTime[:5]
		}
		if len(s.EndTime) > 5 {
			s.EndTime = s.EndTime[:5]
		}
		schedules = append(schedules, s)
	}
	return schedules, nil
}

func containsStr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
