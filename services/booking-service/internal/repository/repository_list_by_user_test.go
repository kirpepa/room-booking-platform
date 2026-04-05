package repository

import (
	"context"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/pashagolub/pgxmock/v3"
)

func TestListByUser_SQLSemanticsPerOpenAPI(t *testing.T) {
	if !strings.Contains(listByUserSQL, "status = 'active'") {
		t.Fatal("cancelled (and other non-active) bookings must not appear in /bookings/my")
	}
	if !strings.Contains(listByUserSQL, "slot_start_at >= NOW()") {
		t.Fatal("only bookings whose slot has started in the future or now (start >= now), per api.yaml")
	}
	if strings.Contains(listByUserSQL, "slot_end_at >") || strings.Contains(listByUserSQL, "slot_end_at>") {
		t.Fatal("must not filter by slot end time; contract is by slot start")
	}
}

func TestListByUser_QueryActiveAndFutureStartOnly(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatal(err)
	}
	defer mock.Close()

	uid := uuid.MustParse("aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa")

	cols := []string{
		"id", "slot_id", "user_id", "room_id", "status", "conference_link",
		"conference_requested", "conference_status", "slot_start_at", "slot_end_at",
		"cancelled_at", "created_at", "updated_at",
	}
	empty := pgxmock.NewRows(cols)

	// api.yaml: future slots by start (start >= now); only meaningful active bookings for the user.
	mock.ExpectQuery(regexp.QuoteMeta(listByUserSQL)).
		WithArgs(uid).
		WillReturnRows(empty)

	repo := New(mock)
	got, err := repo.ListByUser(context.Background(), uid)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("expected no rows, got %d", len(got))
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestListByUser_ScansReturnedRow(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatal(err)
	}
	defer mock.Close()

	uid := uuid.MustParse("bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb")
	bid := uuid.MustParse("cccccccc-cccc-cccc-cccc-cccccccccccc")
	sid := uuid.MustParse("dddddddd-dddd-dddd-dddd-dddddddddddd")
	rid := uuid.MustParse("eeeeeeee-eeee-eeee-eeee-eeeeeeeeeeee")
	now := time.Date(2030, 6, 15, 12, 0, 0, 0, time.UTC)
	start := now
	end := now.Add(30 * time.Minute)

	cols := []string{
		"id", "slot_id", "user_id", "room_id", "status", "conference_link",
		"conference_requested", "conference_status", "slot_start_at", "slot_end_at",
		"cancelled_at", "created_at", "updated_at",
	}
	rows := pgxmock.NewRows(cols).AddRow(
		bid, sid, uid, rid, "active", nil, false, "not_requested",
		start, end, nil, now, now,
	)

	mock.ExpectQuery(regexp.QuoteMeta(listByUserSQL)).
		WithArgs(uid).
		WillReturnRows(rows)

	repo := New(mock)
	got, err := repo.ListByUser(context.Background(), uid)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 row, got %d", len(got))
	}
	if got[0].ID != bid || got[0].Status != "active" || !got[0].SlotStartAt.Equal(start) {
		t.Fatalf("unexpected booking: %+v", got[0])
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}
