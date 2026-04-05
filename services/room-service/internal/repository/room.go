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

var ErrRoomNotFound = errors.New("room not found")

type Room struct {
	ID          uuid.UUID  `json:"id"`
	Name        string     `json:"name"`
	Description *string    `json:"description,omitempty"`
	Capacity    *int       `json:"capacity,omitempty"`
	CreatedBy   uuid.UUID  `json:"-"`
	CreatedAt   time.Time  `json:"createdAt,omitempty"`
	UpdatedAt   time.Time  `json:"-"`
}

type RoomRepository struct {
	pool *pgxpool.Pool
}

func NewRoomRepository(pool *pgxpool.Pool) *RoomRepository {
	return &RoomRepository{pool: pool}
}

func (r *RoomRepository) Create(ctx context.Context, name string, description *string, capacity *int, createdBy uuid.UUID) (*Room, error) {
	id := uuid.New()
	room := &Room{ID: id}
	err := r.pool.QueryRow(ctx,
		`INSERT INTO rooms (id, name, description, capacity, created_by)
		 VALUES ($1, $2, $3, $4, $5)
		 RETURNING id, name, description, capacity, created_by, created_at, updated_at`,
		id, name, description, capacity, createdBy,
	).Scan(&room.ID, &room.Name, &room.Description, &room.Capacity, &room.CreatedBy, &room.CreatedAt, &room.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("insert room: %w", err)
	}
	return room, nil
}

func (r *RoomRepository) List(ctx context.Context) ([]Room, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, name, description, capacity, created_by, created_at, updated_at FROM rooms ORDER BY created_at`)
	if err != nil {
		return nil, fmt.Errorf("list rooms: %w", err)
	}
	defer rows.Close()

	var rooms []Room
	for rows.Next() {
		var room Room
		if err := rows.Scan(&room.ID, &room.Name, &room.Description, &room.Capacity, &room.CreatedBy, &room.CreatedAt, &room.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan room: %w", err)
		}
		rooms = append(rooms, room)
	}
	return rooms, nil
}

func (r *RoomRepository) GetByID(ctx context.Context, id uuid.UUID) (*Room, error) {
	var room Room
	err := r.pool.QueryRow(ctx,
		`SELECT id, name, description, capacity, created_by, created_at, updated_at FROM rooms WHERE id = $1`, id,
	).Scan(&room.ID, &room.Name, &room.Description, &room.Capacity, &room.CreatedBy, &room.CreatedAt, &room.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrRoomNotFound
		}
		return nil, fmt.Errorf("get room: %w", err)
	}
	return &room, nil
}
