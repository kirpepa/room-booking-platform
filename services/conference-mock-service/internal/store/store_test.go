package store

import (
	"sync"
	"testing"

	"github.com/google/uuid"
)

func TestGetOrCreate_Idempotent(t *testing.T) {
	s := New()
	bookingID := uuid.New()

	link1 := s.GetOrCreate(bookingID)
	link2 := s.GetOrCreate(bookingID)

	if link1 != link2 {
		t.Errorf("expected same link, got %s and %s", link1, link2)
	}
}

func TestGetOrCreate_DifferentBookings(t *testing.T) {
	s := New()
	id1 := uuid.New()
	id2 := uuid.New()

	link1 := s.GetOrCreate(id1)
	link2 := s.GetOrCreate(id2)

	if link1 == link2 {
		t.Error("expected different links for different bookings")
	}
}

func TestGetOrCreate_Concurrent(t *testing.T) {
	s := New()
	bookingID := uuid.New()

	var wg sync.WaitGroup
	links := make([]string, 100)

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			links[idx] = s.GetOrCreate(bookingID)
		}(i)
	}
	wg.Wait()

	for i := 1; i < len(links); i++ {
		if links[i] != links[0] {
			t.Errorf("concurrent access produced different links: %s vs %s", links[0], links[i])
		}
	}
}

func TestGetOrCreate_LinkFormat(t *testing.T) {
	s := New()
	bookingID := uuid.New()
	link := s.GetOrCreate(bookingID)

	if link == "" {
		t.Error("expected non-empty link")
	}
	if len(link) < 10 {
		t.Errorf("link too short: %s", link)
	}
}
