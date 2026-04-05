package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/room-booking/services/booking-service/internal/repository"
)

var (
	ErrForbidden      = errors.New("forbidden")
	ErrBookingNotFound = errors.New("booking not found")
	ErrSlotNotFound   = errors.New("slot not found")
	ErrSlotBooked     = errors.New("slot already booked")
	ErrSlotInPast     = errors.New("slot in past")
	ErrAdminCannotBook = errors.New("admin cannot create bookings")
)

// bookingRepository is satisfied by *repository.BookingRepository; narrowed for tests.
type bookingRepository interface {
	Create(ctx context.Context, b *repository.Booking) error
	GetByID(ctx context.Context, id uuid.UUID) (*repository.Booking, error)
	Cancel(ctx context.Context, id uuid.UUID) (*repository.Booking, error)
	ListByUser(ctx context.Context, userID uuid.UUID) ([]repository.Booking, error)
	ListAll(ctx context.Context, page, pageSize int) ([]repository.Booking, int, error)
	CreateConferenceJob(ctx context.Context, bookingID uuid.UUID) error
	GetPendingJobs(ctx context.Context, limit int) ([]repository.ConferenceJob, error)
	UpdateConferenceJob(ctx context.Context, bookingID uuid.UUID, status string, attempts int, nextRetry *time.Time, lastError *string) error
	UpdateConferenceLink(ctx context.Context, bookingID uuid.UUID, link string) error
	UpdateConferenceStatus(ctx context.Context, bookingID uuid.UUID, status string) error
}

type BookingService struct {
	repo                   bookingRepository
	availabilityServiceURL string
	conferenceServiceURL   string
	httpClient             *http.Client
	log                    *slog.Logger
}

func New(repo bookingRepository, availabilityURL, conferenceURL string, log *slog.Logger) *BookingService {
	return &BookingService{
		repo:                   repo,
		availabilityServiceURL: availabilityURL,
		conferenceServiceURL:   conferenceURL,
		httpClient:             &http.Client{Timeout: 10 * time.Second},
		log:                    log,
	}
}

type BookingResponse struct {
	ID             uuid.UUID `json:"id"`
	SlotID         uuid.UUID `json:"slotId"`
	UserID         uuid.UUID `json:"userId"`
	Status         string    `json:"status"`
	ConferenceLink *string   `json:"conferenceLink,omitempty"`
	CreatedAt      time.Time `json:"createdAt,omitempty"`
}

type CreateBookingRequest struct {
	SlotID               uuid.UUID `json:"slotId"`
	CreateConferenceLink bool      `json:"createConferenceLink"`
}

func toResponse(b *repository.Booking) *BookingResponse {
	return &BookingResponse{
		ID:             b.ID,
		SlotID:         b.SlotID,
		UserID:         b.UserID,
		Status:         b.Status,
		ConferenceLink: b.ConferenceLink,
		CreatedAt:      b.CreatedAt,
	}
}

func (s *BookingService) CreateBooking(ctx context.Context, userID uuid.UUID, role string, req CreateBookingRequest) (*BookingResponse, error) {
	if role == "admin" {
		return nil, ErrAdminCannotBook
	}
	if req.SlotID == uuid.Nil {
		return nil, fmt.Errorf("slotId is required")
	}

	bookingID := uuid.New()

	// Book slot in availability-service
	slotMeta, err := s.bookSlot(ctx, req.SlotID, bookingID)
	if err != nil {
		return nil, err
	}

	confStatus := "not_requested"
	if req.CreateConferenceLink {
		confStatus = "pending"
	}

	booking := &repository.Booking{
		ID:                  bookingID,
		SlotID:              req.SlotID,
		UserID:              userID,
		RoomID:              slotMeta.RoomID,
		Status:              "active",
		ConferenceRequested: req.CreateConferenceLink,
		ConferenceStatus:    confStatus,
		SlotStartAt:         slotMeta.StartAt,
		SlotEndAt:           slotMeta.EndAt,
	}

	if err := s.repo.Create(ctx, booking); err != nil {
		s.log.Error("failed to create booking, compensating", "error", err)
		_ = s.releaseSlot(ctx, req.SlotID, bookingID)
		return nil, fmt.Errorf("create booking: %w", err)
	}

	if req.CreateConferenceLink {
		if err := s.repo.CreateConferenceJob(ctx, bookingID); err != nil {
			s.log.Error("failed to create conference job", "error", err)
		}
	}

	created, err := s.repo.GetByID(ctx, bookingID)
	if err != nil {
		return nil, err
	}

	return toResponse(created), nil
}

func (s *BookingService) CancelBooking(ctx context.Context, bookingID, userID uuid.UUID) (*BookingResponse, error) {
	booking, err := s.repo.GetByID(ctx, bookingID)
	if err != nil {
		if errors.Is(err, repository.ErrBookingNotFound) {
			return nil, ErrBookingNotFound
		}
		return nil, err
	}

	if booking.UserID != userID {
		return nil, ErrForbidden
	}

	if booking.Status == "cancelled" {
		return toResponse(booking), nil
	}

	// Release slot
	if err := s.releaseSlot(ctx, booking.SlotID, bookingID); err != nil {
		s.log.Error("release slot failed", "error", err)
	}

	cancelled, err := s.repo.Cancel(ctx, bookingID)
	if err != nil {
		return nil, err
	}

	return toResponse(cancelled), nil
}

func (s *BookingService) GetMyBookings(ctx context.Context, userID uuid.UUID) ([]BookingResponse, error) {
	bookings, err := s.repo.ListByUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	result := make([]BookingResponse, len(bookings))
	for i, b := range bookings {
		result[i] = *toResponse(&b)
	}
	return result, nil
}

func (s *BookingService) ListBookings(ctx context.Context, page, pageSize int) ([]BookingResponse, int, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}

	bookings, total, err := s.repo.ListAll(ctx, page, pageSize)
	if err != nil {
		return nil, 0, err
	}
	result := make([]BookingResponse, len(bookings))
	for i, b := range bookings {
		result[i] = *toResponse(&b)
	}
	return result, total, nil
}

type slotMetaResponse struct {
	RoomID  uuid.UUID `json:"room_id"`
	StartAt time.Time `json:"start_at"`
	EndAt   time.Time `json:"end_at"`
	Status  string    `json:"status"`
}

func (s *BookingService) bookSlot(ctx context.Context, slotID, bookingID uuid.UUID) (*slotMetaResponse, error) {
	body, _ := json.Marshal(map[string]string{
		"slot_id":    slotID.String(),
		"booking_id": bookingID.String(),
	})

	req, _ := http.NewRequestWithContext(ctx, "POST",
		s.availabilityServiceURL+"/internal/slots/book",
		bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("call availability service: %w", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)

	switch resp.StatusCode {
	case http.StatusOK:
		var meta slotMetaResponse
		if err := json.Unmarshal(respBody, &meta); err != nil {
			return nil, fmt.Errorf("decode slot meta: %w", err)
		}
		return &meta, nil
	case http.StatusNotFound:
		return nil, ErrSlotNotFound
	case http.StatusBadRequest:
		return nil, ErrSlotInPast
	case http.StatusConflict:
		return nil, ErrSlotBooked
	default:
		return nil, fmt.Errorf("availability service returned %d: %s", resp.StatusCode, string(respBody))
	}
}

func (s *BookingService) releaseSlot(ctx context.Context, slotID, bookingID uuid.UUID) error {
	body, _ := json.Marshal(map[string]string{
		"slot_id":    slotID.String(),
		"booking_id": bookingID.String(),
	})

	req, _ := http.NewRequestWithContext(ctx, "POST",
		s.availabilityServiceURL+"/internal/slots/release",
		bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("call release: %w", err)
	}
	defer resp.Body.Close()
	return nil
}

// ProcessConferenceJobs is called by the worker
func (s *BookingService) ProcessConferenceJobs(ctx context.Context) error {
	jobs, err := s.repo.GetPendingJobs(ctx, 10)
	if err != nil {
		return fmt.Errorf("get pending jobs: %w", err)
	}

	for _, job := range jobs {
		s.processJob(ctx, job)
	}
	return nil
}

func (s *BookingService) processJob(ctx context.Context, job repository.ConferenceJob) {
	booking, err := s.repo.GetByID(ctx, job.BookingID)
	if err != nil || booking.Status == "cancelled" {
		_ = s.repo.UpdateConferenceJob(ctx, job.BookingID, "done", job.Attempts, nil, nil)
		if booking != nil {
			_ = s.repo.UpdateConferenceStatus(ctx, job.BookingID, "not_requested")
		}
		return
	}

	link, err := s.requestConferenceLink(ctx, job.BookingID)
	if err != nil {
		attempts := job.Attempts + 1
		errStr := err.Error()

		if attempts >= 5 {
			_ = s.repo.UpdateConferenceJob(ctx, job.BookingID, "failed", attempts, nil, &errStr)
			_ = s.repo.UpdateConferenceStatus(ctx, job.BookingID, "failed")
			return
		}

		backoff := time.Duration(1<<uint(attempts)) * time.Second
		nextRetry := time.Now().UTC().Add(backoff)
		_ = s.repo.UpdateConferenceJob(ctx, job.BookingID, "pending", attempts, &nextRetry, &errStr)
		return
	}

	_ = s.repo.UpdateConferenceLink(ctx, job.BookingID, link)
	_ = s.repo.UpdateConferenceJob(ctx, job.BookingID, "done", job.Attempts+1, nil, nil)
}

func (s *BookingService) requestConferenceLink(ctx context.Context, bookingID uuid.UUID) (string, error) {
	body, _ := json.Marshal(map[string]string{
		"booking_id": bookingID.String(),
	})

	req, _ := http.NewRequestWithContext(ctx, "POST",
		s.conferenceServiceURL+"/internal/conference/create",
		bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("conference service returned %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Link string `json:"link"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}
	return result.Link, nil
}
