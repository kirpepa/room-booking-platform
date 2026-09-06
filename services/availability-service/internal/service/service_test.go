package service

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/room-booking/services/availability-service/internal/repository"
)

func TestParseTime_Valid(t *testing.T) {
	tests := []struct {
		input string
		hour  int
		min   int
	}{
		{"09:00", 9, 0},
		{"09:30", 9, 30},
		{"23:59", 23, 59},
		{"00:00", 0, 0},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result, err := parseTime(tt.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result.Hour() != tt.hour || result.Minute() != tt.min {
				t.Errorf("expected %d:%d, got %d:%d", tt.hour, tt.min, result.Hour(), result.Minute())
			}
		})
	}
}

func TestParseTime_Invalid(t *testing.T) {
	tests := []string{
		"",
		"abc",
		"25:00",
		"09:60",
		"9",
		"09:00:00",
		"-1:00",
	}

	for _, tt := range tests {
		t.Run(tt, func(t *testing.T) {
			_, err := parseTime(tt)
			if err == nil {
				t.Error("expected error")
			}
		})
	}
}

func TestCreateScheduleRequest_Validation(t *testing.T) {
	t.Run("empty days", func(t *testing.T) {
		svc := &AvailabilityService{}
		req := CreateScheduleRequest{
			DaysOfWeek: []int{},
			StartTime:  "09:00",
			EndTime:    "17:00",
		}
		_, err := svc.CreateSchedule(nil, uuid.UUID{}, req)
		if err == nil {
			t.Error("expected error for empty days")
		}
	})

	t.Run("invalid day value", func(t *testing.T) {
		svc := &AvailabilityService{}
		req := CreateScheduleRequest{
			DaysOfWeek: []int{0, 1},
			StartTime:  "09:00",
			EndTime:    "17:00",
		}
		_, err := svc.CreateSchedule(nil, uuid.UUID{}, req)
		if err == nil {
			t.Error("expected error for day=0")
		}
	})

	t.Run("day > 7", func(t *testing.T) {
		svc := &AvailabilityService{}
		req := CreateScheduleRequest{
			DaysOfWeek: []int{8},
			StartTime:  "09:00",
			EndTime:    "17:00",
		}
		_, err := svc.CreateSchedule(nil, uuid.UUID{}, req)
		if err == nil {
			t.Error("expected error for day=8")
		}
	})
}

func TestTimeDurationDivisibility(t *testing.T) {
	tests := []struct {
		start string
		end   string
		valid bool
	}{
		{"09:00", "10:00", true},  // 60 min, divisible by 30
		{"09:00", "09:30", true},  // 30 min
		{"09:00", "10:30", true},  // 90 min
		{"09:00", "09:15", false}, // 15 min, not divisible
		{"09:00", "09:45", false}, // 45 min, not divisible
	}

	for _, tt := range tests {
		t.Run(tt.start+"-"+tt.end, func(t *testing.T) {
			startTime, _ := parseTime(tt.start)
			endTime, _ := parseTime(tt.end)
			dur := endTime.Sub(startTime)
			divisible := int(dur.Minutes())%30 == 0
			if divisible != tt.valid {
				t.Errorf("expected divisible=%v for %s-%s", tt.valid, tt.start, tt.end)
			}
		})
	}
}

type fakeAvailRepo struct {
	sched          *repository.Schedule
	getSchedErr    error
	freeSlots      []repository.Slot
	freeSlotsErr   error
	createSchedErr error
	genCount       int
	genErr         error
	upsertErr      error
	upsertCalls    int
	genState       *time.Time
	genStateErr    error
	allSchedules   []repository.Schedule
	allSchedErr    error
	bookMeta       *repository.SlotMeta
	bookErr        error
	releaseErr     error
	slotByID       *repository.Slot
	slotByIDErr    error
}

func (f *fakeAvailRepo) CreateSchedule(ctx context.Context, id, roomID uuid.UUID, daysOfWeek []int16, startTime, endTime string) (*repository.Schedule, error) {
	if f.createSchedErr != nil {
		return nil, f.createSchedErr
	}
	return &repository.Schedule{
		ID: roomID, RoomID: roomID, DaysOfWeek: daysOfWeek, StartTime: startTime, EndTime: endTime,
	}, nil
}

func (f *fakeAvailRepo) GetScheduleByRoomID(ctx context.Context, roomID uuid.UUID) (*repository.Schedule, error) {
	if f.getSchedErr != nil {
		return nil, f.getSchedErr
	}
	if f.sched != nil {
		return f.sched, nil
	}
	return &repository.Schedule{RoomID: roomID}, nil
}

func (f *fakeAvailRepo) GetFreeSlots(ctx context.Context, roomID uuid.UUID, date time.Time) ([]repository.Slot, error) {
	if f.freeSlotsErr != nil {
		return nil, f.freeSlotsErr
	}
	return f.freeSlots, nil
}

func (f *fakeAvailRepo) GenerateSlots(ctx context.Context, schedule *repository.Schedule, from, until time.Time) (int, error) {
	if f.genErr != nil {
		return 0, f.genErr
	}
	return f.genCount, nil
}

func (f *fakeAvailRepo) UpsertGenerationState(ctx context.Context, roomID uuid.UUID, until time.Time) error {
	f.upsertCalls++
	return f.upsertErr
}

func (f *fakeAvailRepo) GetGenerationState(ctx context.Context, roomID uuid.UUID) (*time.Time, error) {
	if f.genStateErr != nil {
		return nil, f.genStateErr
	}
	return f.genState, nil
}

func (f *fakeAvailRepo) GetAllSchedules(ctx context.Context) ([]repository.Schedule, error) {
	if f.allSchedErr != nil {
		return nil, f.allSchedErr
	}
	return f.allSchedules, nil
}

func (f *fakeAvailRepo) BookSlot(ctx context.Context, slotID, bookingID uuid.UUID) (*repository.SlotMeta, error) {
	if f.bookErr != nil {
		return nil, f.bookErr
	}
	return f.bookMeta, nil
}

func (f *fakeAvailRepo) ReleaseSlot(ctx context.Context, slotID, bookingID uuid.UUID) error {
	return f.releaseErr
}

func (f *fakeAvailRepo) GetSlotByID(ctx context.Context, slotID uuid.UUID) (*repository.Slot, error) {
	if f.slotByIDErr != nil {
		return nil, f.slotByIDErr
	}
	return f.slotByID, nil
}

func discardLog() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestGetFreeSlots_InvalidDate(t *testing.T) {
	s := &AvailabilityService{repo: &fakeAvailRepo{}, log: discardLog(), roomServiceURL: ""}
	_, err := s.GetFreeSlots(context.Background(), uuid.New(), "not-a-date")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestGetFreeSlots_NoSchedule(t *testing.T) {
	rid := uuid.New()
	s := &AvailabilityService{
		repo:           &fakeAvailRepo{getSchedErr: repository.ErrScheduleNotFound},
		log:            discardLog(),
		roomServiceURL: "",
	}
	slots, err := s.GetFreeSlots(context.Background(), rid, "2030-06-15")
	if err != nil {
		t.Fatal(err)
	}
	if len(slots) != 0 {
		t.Fatalf("expected empty, got %d", len(slots))
	}
}

func TestGetFreeSlots_FiltersPastSlots(t *testing.T) {
	rid := uuid.New()
	past := time.Now().UTC().Add(-2 * time.Hour)
	future := time.Now().UTC().Add(48 * time.Hour)
	s := &AvailabilityService{
		repo: &fakeAvailRepo{
			sched: &repository.Schedule{ID: uuid.New(), RoomID: rid},
			freeSlots: []repository.Slot{
				{StartAt: past, EndAt: past.Add(30 * time.Minute)},
				{StartAt: future, EndAt: future.Add(30 * time.Minute)},
			},
		},
		log:            discardLog(),
		roomServiceURL: "",
	}
	slots, err := s.GetFreeSlots(context.Background(), rid, "2030-06-15")
	if err != nil {
		t.Fatal(err)
	}
	if len(slots) != 1 {
		t.Fatalf("expected 1 future slot, got %d", len(slots))
	}
}

func TestCreateSchedule_RoomIDMismatch(t *testing.T) {
	s := &AvailabilityService{repo: &fakeAvailRepo{}, log: discardLog(), roomServiceURL: ""}
	pathID := uuid.New()
	req := CreateScheduleRequest{
		RoomID:     uuid.New(),
		DaysOfWeek: []int{1},
		StartTime:  "09:00",
		EndTime:    "10:00",
	}
	_, err := s.CreateSchedule(context.Background(), pathID, req)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestCreateSchedule_ScheduleExists(t *testing.T) {
	rid := uuid.New()
	s := &AvailabilityService{
		repo:           &fakeAvailRepo{createSchedErr: repository.ErrScheduleExists},
		log:            discardLog(),
		roomServiceURL: "",
	}
	req := CreateScheduleRequest{RoomID: rid, DaysOfWeek: []int{1}, StartTime: "09:00", EndTime: "10:00"}
	_, err := s.CreateSchedule(context.Background(), rid, req)
	if err != ErrScheduleExists {
		t.Fatalf("got %v", err)
	}
}

func TestExtendSlots_GeneratesAndUpserts(t *testing.T) {
	rid := uuid.New()
	fr := &fakeAvailRepo{
		allSchedules: []repository.Schedule{{ID: uuid.New(), RoomID: rid, StartTime: "09:00", EndTime: "10:00"}},
		genCount:     3,
	}
	s := &AvailabilityService{repo: fr, log: discardLog(), roomServiceURL: ""}
	if err := s.ExtendSlots(context.Background()); err != nil {
		t.Fatal(err)
	}
	if fr.upsertCalls != 1 {
		t.Fatalf("expected generation state update, got %d", fr.upsertCalls)
	}
}

func TestCreateSchedule_DoesNotAdvanceStateAfterGenerationFailure(t *testing.T) {
	rid := uuid.New()
	fr := &fakeAvailRepo{genErr: io.ErrUnexpectedEOF}
	s := &AvailabilityService{repo: fr, log: discardLog(), roomServiceURL: ""}
	req := CreateScheduleRequest{RoomID: rid, DaysOfWeek: []int{1}, StartTime: "09:00", EndTime: "10:00"}

	if _, err := s.CreateSchedule(context.Background(), rid, req); err != nil {
		t.Fatal(err)
	}
	if fr.upsertCalls != 0 {
		t.Fatal("generation state advanced despite failed slot generation")
	}
}

func TestExtendSlots_ReportsPerScheduleFailures(t *testing.T) {
	rid := uuid.New()
	fr := &fakeAvailRepo{
		allSchedules: []repository.Schedule{{ID: uuid.New(), RoomID: rid, StartTime: "09:00", EndTime: "10:00"}},
		genErr:       io.ErrUnexpectedEOF,
	}
	s := &AvailabilityService{repo: fr, log: discardLog(), roomServiceURL: ""}
	if err := s.ExtendSlots(context.Background()); err == nil {
		t.Fatal("expected generation failure to reach the worker")
	}
	if fr.upsertCalls != 0 {
		t.Fatal("generation state advanced despite failed slot generation")
	}
}

func TestGetFreeSlots_RoomServiceReturns404(t *testing.T) {
	rid := uuid.New()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method %s", r.Method)
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()
	s := &AvailabilityService{
		repo:           &fakeAvailRepo{},
		log:            discardLog(),
		roomServiceURL: ts.URL,
		httpClient:     ts.Client(),
	}
	_, err := s.GetFreeSlots(context.Background(), rid, "2030-04-01")
	if err != ErrRoomNotFound {
		t.Fatalf("got %v", err)
	}
}

func TestCreateSchedule_RoomServiceReturns404(t *testing.T) {
	rid := uuid.New()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()
	s := &AvailabilityService{
		repo:           &fakeAvailRepo{},
		log:            discardLog(),
		roomServiceURL: ts.URL,
		httpClient:     ts.Client(),
	}
	req := CreateScheduleRequest{RoomID: rid, DaysOfWeek: []int{1}, StartTime: "09:00", EndTime: "10:00"}
	_, err := s.CreateSchedule(context.Background(), rid, req)
	if err != ErrRoomNotFound {
		t.Fatalf("got %v", err)
	}
}

func TestCreateSchedule_SuccessWithRoomHTTPCheck(t *testing.T) {
	rid := uuid.New()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()
	fr := &fakeAvailRepo{genCount: 1}
	s := &AvailabilityService{
		repo:           fr,
		log:            discardLog(),
		roomServiceURL: ts.URL,
		httpClient:     ts.Client(),
	}
	req := CreateScheduleRequest{RoomID: rid, DaysOfWeek: []int{1}, StartTime: "09:00", EndTime: "10:00"}
	sched, err := s.CreateSchedule(context.Background(), rid, req)
	if err != nil || sched == nil {
		t.Fatalf("CreateSchedule: %v", err)
	}
}

func TestExtendSlots_GetAllSchedulesError(t *testing.T) {
	s := &AvailabilityService{
		repo:           &fakeAvailRepo{allSchedErr: io.ErrClosedPipe},
		log:            discardLog(),
		roomServiceURL: "",
	}
	if err := s.ExtendSlots(context.Background()); err == nil {
		t.Fatal("expected error")
	}
}

func TestBookSlot_ReleaseSlot_GetSlot_Delegate(t *testing.T) {
	sid, bid := uuid.New(), uuid.New()
	meta := &repository.SlotMeta{SlotID: sid, RoomID: uuid.New()}
	slot := &repository.Slot{ID: sid}
	fr := &fakeAvailRepo{bookMeta: meta, slotByID: slot}
	s := &AvailabilityService{repo: fr, log: discardLog(), roomServiceURL: ""}

	got, err := s.BookSlot(context.Background(), sid, bid)
	if err != nil || got != meta {
		t.Fatalf("BookSlot: %v", err)
	}
	if err := s.ReleaseSlot(context.Background(), sid, bid); err != nil {
		t.Fatal(err)
	}
	gotSlot, err := s.GetSlot(context.Background(), sid)
	if err != nil || gotSlot != slot {
		t.Fatalf("GetSlot: %v", err)
	}
}
