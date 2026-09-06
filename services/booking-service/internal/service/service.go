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
	"net/url"
	"time"

	"github.com/google/uuid"
	"github.com/room-booking/services/booking-service/internal/repository"
)

var (
	ErrForbidden       = errors.New("forbidden")
	ErrBookingNotFound = errors.New("booking not found")
	ErrSlotNotFound    = errors.New("slot not found")
	ErrSlotBooked      = errors.New("slot already booked")
	ErrSlotInPast      = errors.New("slot in past")
	ErrAdminCannotBook = errors.New("admin cannot create bookings")
)

// bookingRepository is satisfied by *repository.BookingRepository; narrowed for tests.
type bookingRepository interface {
	CreateWithConferenceJob(ctx context.Context, b *repository.Booking) error
	GetByID(ctx context.Context, id uuid.UUID) (*repository.Booking, error)
	Cancel(ctx context.Context, id uuid.UUID) (*repository.Booking, error)
	ListByUser(ctx context.Context, userID uuid.UUID) ([]repository.Booking, error)
	ListAll(ctx context.Context, page, pageSize int) ([]repository.Booking, int, error)
	ClaimConferenceJobs(ctx context.Context, limit int, lease time.Duration) ([]repository.ConferenceJob, error)
	RescheduleConferenceJob(ctx context.Context, bookingID uuid.UUID, attempts int, nextRetry time.Time, lastError string) error
	CompleteConferenceJob(ctx context.Context, bookingID uuid.UUID, link string, attempts int) error
	FailConferenceJob(ctx context.Context, bookingID uuid.UUID, attempts int, lastError string) error
	DiscardConferenceJob(ctx context.Context, bookingID uuid.UUID, attempts int) error
}

const (
	conferenceJobBatchSize = 10
	conferenceJobLease     = 5 * time.Minute
	maxConferenceAttempts  = 5
	maxResponseBodyBytes   = 1 << 20
)

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

	if err := s.repo.CreateWithConferenceJob(ctx, booking); err != nil {
		if errors.Is(err, repository.ErrCommitOutcomeUnknown) {
			persisted, lookupErr := s.repo.GetByID(ctx, bookingID)
			if lookupErr == nil {
				return toResponse(persisted), nil
			}
			if !errors.Is(lookupErr, repository.ErrBookingNotFound) {
				s.log.Error("booking commit outcome remains unknown", "error", err, "lookupError", lookupErr, "bookingId", bookingID)
				return nil, fmt.Errorf("create booking outcome is unknown: %w", errors.Join(err, lookupErr))
			}
		}
		s.log.Error("failed to create booking, compensating", "error", err)
		if releaseErr := s.releaseSlot(ctx, req.SlotID, bookingID); releaseErr != nil {
			s.log.Error("booking compensation failed", "error", releaseErr, "bookingId", bookingID)
			return nil, fmt.Errorf("create booking and compensate slot: %w", errors.Join(err, releaseErr))
		}
		return nil, fmt.Errorf("create booking: %w", err)
	}

	return toResponse(booking), nil
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
		return nil, fmt.Errorf("release slot: %w", err)
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
	body, err := json.Marshal(map[string]string{
		"slot_id":    slotID.String(),
		"booking_id": bookingID.String(),
	})
	if err != nil {
		return nil, fmt.Errorf("encode book slot request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST",
		s.availabilityServiceURL+"/internal/slots/book",
		bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create book slot request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("call availability service: %w", err)
	}
	defer resp.Body.Close()
	respBody, err := readResponseBody(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read availability response: %w", err)
	}

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
	body, err := json.Marshal(map[string]string{
		"slot_id":    slotID.String(),
		"booking_id": bookingID.String(),
	})
	if err != nil {
		return fmt.Errorf("encode release slot request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST",
		s.availabilityServiceURL+"/internal/slots/release",
		bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create release slot request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("call release: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, readErr := readResponseBody(resp.Body)
		if readErr != nil {
			return fmt.Errorf("release service returned %d; read response: %w", resp.StatusCode, readErr)
		}
		return fmt.Errorf("release service returned %d: %s", resp.StatusCode, string(body))
	}
	return nil
}

// ProcessConferenceJobs is called by the worker
func (s *BookingService) ProcessConferenceJobs(ctx context.Context) error {
	jobs, err := s.repo.ClaimConferenceJobs(ctx, conferenceJobBatchSize, conferenceJobLease)
	if err != nil {
		return fmt.Errorf("claim pending jobs: %w", err)
	}

	var processErrors []error
	for _, job := range jobs {
		if err := s.processJob(ctx, job); err != nil {
			s.log.Error("process conference job failed", "bookingId", job.BookingID, "error", err)
			processErrors = append(processErrors, err)
		}
	}
	return errors.Join(processErrors...)
}

func (s *BookingService) processJob(ctx context.Context, job repository.ConferenceJob) error {
	booking, err := s.repo.GetByID(ctx, job.BookingID)
	if err != nil {
		return fmt.Errorf("load booking %s: %w", job.BookingID, err)
	}
	if booking.Status == "cancelled" {
		if err := s.repo.DiscardConferenceJob(ctx, job.BookingID, job.Attempts); err != nil {
			return fmt.Errorf("discard conference job %s: %w", job.BookingID, err)
		}
		return nil
	}

	link, err := s.requestConferenceLink(ctx, job.BookingID)
	if err != nil {
		attempts := job.Attempts + 1
		errStr := err.Error()

		if attempts >= maxConferenceAttempts {
			if updateErr := s.repo.FailConferenceJob(ctx, job.BookingID, attempts, errStr); updateErr != nil {
				return fmt.Errorf("record terminal conference failure for %s: %w", job.BookingID, updateErr)
			}
			return nil
		}

		backoff := time.Duration(1<<uint(attempts)) * time.Second
		nextRetry := time.Now().UTC().Add(backoff)
		if updateErr := s.repo.RescheduleConferenceJob(ctx, job.BookingID, attempts, nextRetry, errStr); updateErr != nil {
			return fmt.Errorf("reschedule conference job %s: %w", job.BookingID, updateErr)
		}
		return nil
	}

	if err := s.repo.CompleteConferenceJob(ctx, job.BookingID, link, job.Attempts+1); err != nil {
		return fmt.Errorf("complete conference job %s: %w", job.BookingID, err)
	}
	return nil
}

func (s *BookingService) requestConferenceLink(ctx context.Context, bookingID uuid.UUID) (string, error) {
	body, err := json.Marshal(map[string]string{
		"booking_id": bookingID.String(),
	})
	if err != nil {
		return "", fmt.Errorf("encode conference request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST",
		s.conferenceServiceURL+"/internal/conference/create",
		bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("create conference request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, readErr := readResponseBody(resp.Body)
		if readErr != nil {
			return "", fmt.Errorf("conference service returned %d; read response: %w", resp.StatusCode, readErr)
		}
		return "", fmt.Errorf("conference service returned %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Link string `json:"link"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}
	parsed, err := url.ParseRequestURI(result.Link)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
		return "", errors.New("conference service returned an invalid HTTPS link")
	}
	return result.Link, nil
}

func readResponseBody(body io.Reader) ([]byte, error) {
	limited := io.LimitReader(body, maxResponseBodyBytes+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return nil, err
	}
	if len(data) > maxResponseBodyBytes {
		return nil, errors.New("response body exceeds 1 MiB")
	}
	return data, nil
}
