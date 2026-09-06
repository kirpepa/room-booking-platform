package repository

import (
	"context"
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/pashagolub/pgxmock/v3"
)

func testBooking() *Booking {
	start := time.Date(2030, 1, 2, 12, 0, 0, 0, time.UTC)
	return &Booking{
		ID:                  uuid.New(),
		SlotID:              uuid.New(),
		UserID:              uuid.New(),
		RoomID:              uuid.New(),
		Status:              "active",
		ConferenceRequested: true,
		ConferenceStatus:    "pending",
		SlotStartAt:         start,
		SlotEndAt:           start.Add(30 * time.Minute),
	}
}

func expectBookingInsert(mock pgxmock.PgxPoolIface, booking *Booking, createdAt time.Time) {
	mock.ExpectQuery(regexp.QuoteMeta(`INSERT INTO bookings (id, slot_id, user_id, room_id, status, conference_requested, conference_status, slot_start_at, slot_end_at)`)).
		WithArgs(
			booking.ID, booking.SlotID, booking.UserID, booking.RoomID, booking.Status,
			booking.ConferenceRequested, booking.ConferenceStatus, booking.SlotStartAt, booking.SlotEndAt,
		).
		WillReturnRows(pgxmock.NewRows([]string{"created_at", "updated_at"}).AddRow(createdAt, createdAt))
}

func TestCreateWithConferenceJob_CommitsBothWrites(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatal(err)
	}
	defer mock.Close()

	booking := testBooking()
	createdAt := time.Now().UTC().Truncate(time.Second)
	mock.ExpectBegin()
	expectBookingInsert(mock, booking, createdAt)
	mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO conference_jobs (booking_id, status) VALUES ($1, 'pending')`)).
		WithArgs(booking.ID).
		WillReturnResult(pgxmock.NewResult("INSERT", 1))
	mock.ExpectCommit()

	if err := New(mock).CreateWithConferenceJob(context.Background(), booking); err != nil {
		t.Fatal(err)
	}
	if !booking.CreatedAt.Equal(createdAt) {
		t.Fatalf("created timestamp was not returned: %s", booking.CreatedAt)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestCreateWithConferenceJob_RollsBackIfJobInsertFails(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatal(err)
	}
	defer mock.Close()

	booking := testBooking()
	mock.ExpectBegin()
	expectBookingInsert(mock, booking, time.Now().UTC())
	mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO conference_jobs (booking_id, status) VALUES ($1, 'pending')`)).
		WithArgs(booking.ID).
		WillReturnError(errors.New("database unavailable"))
	mock.ExpectRollback()

	if err := New(mock).CreateWithConferenceJob(context.Background(), booking); err == nil {
		t.Fatal("expected the transaction to fail")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestClaimConferenceJobs_ClaimsInsideTransaction(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatal(err)
	}
	defer mock.Close()

	bookingID := uuid.New()
	mock.ExpectBegin()
	mock.ExpectQuery("WITH candidates AS").
		WithArgs(2, pgxmock.AnyArg()).
		WillReturnRows(pgxmock.NewRows([]string{
			"booking_id", "status", "attempts", "next_retry_at", "last_error",
		}).AddRow(bookingID, "processing", 1, nil, nil))
	mock.ExpectCommit()

	jobs, err := New(mock).ClaimConferenceJobs(context.Background(), 2, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 1 || jobs[0].BookingID != bookingID || jobs[0].Status != "processing" {
		t.Fatalf("unexpected claimed jobs: %+v", jobs)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestCompleteConferenceJob_RollsBackBothUpdatesOnFailure(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatal(err)
	}
	defer mock.Close()

	bookingID := uuid.New()
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE bookings").
		WithArgs(bookingID, "https://meet.example/room").
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	mock.ExpectExec("UPDATE conference_jobs").
		WithArgs(bookingID, "done", 2, pgxmock.AnyArg()).
		WillReturnError(errors.New("write failed"))
	mock.ExpectRollback()

	err = New(mock).CompleteConferenceJob(context.Background(), bookingID, "https://meet.example/room", 2)
	if err == nil {
		t.Fatal("expected completion to fail")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
