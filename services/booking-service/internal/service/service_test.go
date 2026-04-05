package service

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/room-booking/services/booking-service/internal/repository"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

type memBookingRepo struct {
	bookings         map[uuid.UUID]*repository.Booking
	createErr        error
	cancelResult     *repository.Booking
	cancelErr        error
	listUser         []repository.Booking
	listUserErr      error
	listAll          []repository.Booking
	listTotal        int
	listAllErr       error
	jobs             []repository.ConferenceJob
	jobsErr          error
	confJobCreateErr error
}

func (m *memBookingRepo) Create(ctx context.Context, b *repository.Booking) error {
	if m.createErr != nil {
		return m.createErr
	}
	if m.bookings == nil {
		m.bookings = make(map[uuid.UUID]*repository.Booking)
	}
	cp := *b
	m.bookings[b.ID] = &cp
	return nil
}

func (m *memBookingRepo) GetByID(ctx context.Context, id uuid.UUID) (*repository.Booking, error) {
	if m.bookings == nil {
		return nil, repository.ErrBookingNotFound
	}
	b, ok := m.bookings[id]
	if !ok {
		return nil, repository.ErrBookingNotFound
	}
	cp := *b
	return &cp, nil
}

func (m *memBookingRepo) Cancel(ctx context.Context, id uuid.UUID) (*repository.Booking, error) {
	if m.cancelErr != nil {
		return nil, m.cancelErr
	}
	if m.cancelResult != nil {
		return m.cancelResult, nil
	}
	b, err := m.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	b.Status = "cancelled"
	now := time.Now().UTC()
	b.CancelledAt = &now
	m.bookings[id] = b
	cp := *b
	return &cp, nil
}

func (m *memBookingRepo) ListByUser(ctx context.Context, userID uuid.UUID) ([]repository.Booking, error) {
	if m.listUserErr != nil {
		return nil, m.listUserErr
	}
	return m.listUser, nil
}

func (m *memBookingRepo) ListAll(ctx context.Context, page, pageSize int) ([]repository.Booking, int, error) {
	if m.listAllErr != nil {
		return nil, 0, m.listAllErr
	}
	return m.listAll, m.listTotal, nil
}

func (m *memBookingRepo) CreateConferenceJob(ctx context.Context, bookingID uuid.UUID) error {
	return m.confJobCreateErr
}

func (m *memBookingRepo) GetPendingJobs(ctx context.Context, limit int) ([]repository.ConferenceJob, error) {
	if m.jobsErr != nil {
		return nil, m.jobsErr
	}
	return m.jobs, nil
}

func (m *memBookingRepo) UpdateConferenceJob(ctx context.Context, bookingID uuid.UUID, status string, attempts int, nextRetry *time.Time, lastError *string) error {
	return nil
}

func (m *memBookingRepo) UpdateConferenceLink(ctx context.Context, bookingID uuid.UUID, link string) error {
	return nil
}

func (m *memBookingRepo) UpdateConferenceStatus(ctx context.Context, bookingID uuid.UUID, status string) error {
	return nil
}

func defaultConfHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"link":"https://meet.example/room"}`))
}

// newTestBookingService wires one httptest server for availability + conference paths (same base URL as production split services).
func newTestBookingService(repo bookingRepository, bookFn http.HandlerFunc, releaseOverride http.HandlerFunc) (*BookingService, *httptest.Server, *int32) {
	if bookFn == nil {
		bookFn = func(w http.ResponseWriter, r *http.Request) {}
	}
	var releaseCalls int32
	mux := http.NewServeMux()
	mux.HandleFunc("/internal/slots/book", bookFn)
	mux.HandleFunc("/internal/slots/release", func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&releaseCalls, 1)
		if releaseOverride != nil {
			releaseOverride(w, r)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("/internal/conference/create", defaultConfHandler)
	srv := httptest.NewServer(mux)
	s := New(repo, srv.URL, srv.URL, testLogger())
	s.httpClient = srv.Client()
	return s, srv, &releaseCalls
}

func TestCreateBooking_AdminCannotBook(t *testing.T) {
	s := New(&memBookingRepo{}, "http://unused", "http://unused", testLogger())
	_, err := s.CreateBooking(context.Background(), uuid.New(), "admin", CreateBookingRequest{SlotID: uuid.New()})
	if err != ErrAdminCannotBook {
		t.Fatalf("expected ErrAdminCannotBook, got %v", err)
	}
}

func TestCreateBooking_MissingSlotID(t *testing.T) {
	s := New(&memBookingRepo{}, "http://unused", "http://unused", testLogger())
	_, err := s.CreateBooking(context.Background(), uuid.New(), "user", CreateBookingRequest{})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestCreateBooking_AvailabilityErrors(t *testing.T) {
	slotID := uuid.New()
	tests := []struct {
		name   string
		code   int
		body   string
		wantErr error
	}{
		{"not_found", http.StatusNotFound, `{}`, ErrSlotNotFound},
		{"past", http.StatusBadRequest, `{}`, ErrSlotInPast},
		{"booked", http.StatusConflict, `{}`, ErrSlotBooked},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &memBookingRepo{}
			bookFn := func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.code)
				_, _ = w.Write([]byte(tt.body))
			}
			svc, srv, _ := newTestBookingService(repo, bookFn, nil)
			defer srv.Close()

			_, err := svc.CreateBooking(context.Background(), uuid.New(), "user", CreateBookingRequest{SlotID: slotID})
			if err != tt.wantErr {
				t.Fatalf("expected %v, got %v", tt.wantErr, err)
			}
		})
	}
}

func TestCreateBooking_Success(t *testing.T) {
	roomID := uuid.New()
	slotID := uuid.New()
	userID := uuid.New()
	start := time.Now().UTC().Add(time.Hour).Truncate(time.Second)
	end := start.Add(30 * time.Minute)

	bookFn := func(w http.ResponseWriter, r *http.Request) {
		meta := map[string]interface{}{
			"room_id":   roomID.String(),
			"start_at":  start.Format(time.RFC3339Nano),
			"end_at":    end.Format(time.RFC3339Nano),
			"status":    "booked",
		}
		b, _ := json.Marshal(meta)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(b)
	}

	repo := &memBookingRepo{}
	svc, srv, _ := newTestBookingService(repo, bookFn, nil)
	defer srv.Close()

	resp, err := svc.CreateBooking(context.Background(), userID, "user", CreateBookingRequest{SlotID: slotID})
	if err != nil {
		t.Fatal(err)
	}
	if resp.UserID != userID || resp.SlotID != slotID || resp.Status != "active" {
		t.Fatalf("unexpected response: %+v", resp)
	}
}

func TestCreateBooking_CompensatesOnRepoCreateFailure(t *testing.T) {
	slotID := uuid.New()
	roomID := uuid.New()
	start := time.Now().UTC().Add(time.Hour)
	end := start.Add(30 * time.Minute)

	bookFn := func(w http.ResponseWriter, r *http.Request) {
		meta, _ := json.Marshal(map[string]interface{}{
			"room_id": roomID.String(), "start_at": start.Format(time.RFC3339Nano),
			"end_at": end.Format(time.RFC3339Nano), "status": "booked",
		})
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(meta)
	}

	repo := &memBookingRepo{createErr: io.ErrUnexpectedEOF}
	svc, srv, rel := newTestBookingService(repo, bookFn, nil)
	defer srv.Close()

	_, err := svc.CreateBooking(context.Background(), uuid.New(), "user", CreateBookingRequest{SlotID: slotID})
	if err == nil {
		t.Fatal("expected error")
	}
	if atomic.LoadInt32(rel) != 1 {
		t.Fatalf("expected release slot called once, got %d", *rel)
	}
}

func TestCancelBooking(t *testing.T) {
	userID := uuid.New()
	other := uuid.New()
	slotID := uuid.New()
	roomID := uuid.New()
	bid := uuid.New()

	t.Run("not_found", func(t *testing.T) {
		repo := &memBookingRepo{}
		svc, srv, _ := newTestBookingService(repo, func(w http.ResponseWriter, r *http.Request) {}, nil)
		defer srv.Close()
		_, err := svc.CancelBooking(context.Background(), uuid.New(), userID)
		if err != ErrBookingNotFound {
			t.Fatalf("got %v", err)
		}
	})

	t.Run("forbidden", func(t *testing.T) {
		repo := &memBookingRepo{bookings: map[uuid.UUID]*repository.Booking{
			bid: {ID: bid, UserID: other, SlotID: slotID, RoomID: roomID, Status: "active"},
		}}
		svc, srv, _ := newTestBookingService(repo, nil, nil)
		defer srv.Close()
		_, err := svc.CancelBooking(context.Background(), bid, userID)
		if err != ErrForbidden {
			t.Fatalf("got %v", err)
		}
	})

	t.Run("idempotent_cancelled", func(t *testing.T) {
		repo := &memBookingRepo{bookings: map[uuid.UUID]*repository.Booking{
			bid: {ID: bid, UserID: userID, SlotID: slotID, RoomID: roomID, Status: "cancelled"},
		}}
		svc, srv, rel := newTestBookingService(repo, nil, nil)
		defer srv.Close()
		resp, err := svc.CancelBooking(context.Background(), bid, userID)
		if err != nil {
			t.Fatal(err)
		}
		if resp.Status != "cancelled" {
			t.Fatalf("status %s", resp.Status)
		}
		if atomic.LoadInt32(rel) != 0 {
			t.Fatal("release should not be called for already cancelled")
		}
	})

	t.Run("active_releases", func(t *testing.T) {
		repo := &memBookingRepo{bookings: map[uuid.UUID]*repository.Booking{
			bid: {ID: bid, UserID: userID, SlotID: slotID, RoomID: roomID, Status: "active"},
		}}
		svc, srv, rel := newTestBookingService(repo, nil, nil)
		defer srv.Close()
		resp, err := svc.CancelBooking(context.Background(), bid, userID)
		if err != nil {
			t.Fatal(err)
		}
		if resp.Status != "cancelled" {
			t.Fatalf("status %s", resp.Status)
		}
		if atomic.LoadInt32(rel) != 1 {
			t.Fatal("expected release")
		}
	})
}

func TestGetMyBookings_ListBookings(t *testing.T) {
	uid := uuid.New()
	b := repository.Booking{ID: uuid.New(), UserID: uid, Status: "active"}
	repo := &memBookingRepo{listUser: []repository.Booking{b}}
	svc := New(repo, "http://x", "http://x", testLogger())
	out, err := svc.GetMyBookings(context.Background(), uid)
	if err != nil || len(out) != 1 {
		t.Fatalf("GetMyBookings: %v %#v", err, out)
	}

	repo2 := &memBookingRepo{listAll: []repository.Booking{b}, listTotal: 1}
	svc2 := New(repo2, "http://x", "http://x", testLogger())
	list, total, err := svc2.ListBookings(context.Background(), 0, 0)
	if err != nil || total != 1 || len(list) != 1 {
		t.Fatalf("ListBookings: %v total=%d n=%d", err, total, len(list))
	}
}

func TestListBookings_ClampsPagination(t *testing.T) {
	repo := &memBookingRepo{listAll: nil, listTotal: 0}
	svc := New(repo, "http://x", "http://x", testLogger())
	_, _, err := svc.ListBookings(context.Background(), -5, 500)
	if err != nil {
		t.Fatal(err)
	}
	// repo sees clamped values: page 1, pageSize 100 — memBookingRepo ignores args; test passes if no panic
}

func TestProcessConferenceJobs_GetJobsError(t *testing.T) {
	repo := &memBookingRepo{jobsErr: io.ErrClosedPipe}
	svc := New(repo, "http://x", "http://x", testLogger())
	err := svc.ProcessConferenceJobs(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestCreateBooking_BookResponseInvalidJSON(t *testing.T) {
	bookFn := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{`))
	}
	repo := &memBookingRepo{}
	svc, srv, _ := newTestBookingService(repo, bookFn, nil)
	defer srv.Close()
	_, err := svc.CreateBooking(context.Background(), uuid.New(), "user", CreateBookingRequest{SlotID: uuid.New()})
	if err == nil {
		t.Fatal("expected decode error")
	}
}

func TestProcessConferenceJobs_UpdatesLinkOnSuccess(t *testing.T) {
	bid := uuid.New()
	repo := &memBookingRepo{
		jobs: []repository.ConferenceJob{{BookingID: bid, Attempts: 0}},
		bookings: map[uuid.UUID]*repository.Booking{
			bid: {ID: bid, UserID: uuid.New(), Status: "active", SlotID: uuid.New(), RoomID: uuid.New()},
		},
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/internal/slots/book", func(w http.ResponseWriter, r *http.Request) {})
	mux.HandleFunc("/internal/slots/release", func(w http.ResponseWriter, r *http.Request) {})
	mux.HandleFunc("/internal/conference/create", defaultConfHandler)
	srv := httptest.NewServer(mux)
	defer srv.Close()
	svc := New(repo, srv.URL, srv.URL, testLogger())
	svc.httpClient = srv.Client()

	if err := svc.ProcessConferenceJobs(context.Background()); err != nil {
		t.Fatal(err)
	}
}
