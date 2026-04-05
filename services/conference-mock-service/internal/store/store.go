package store

import (
	"fmt"
	"sync"

	"github.com/google/uuid"
)

type Store struct {
	mu    sync.RWMutex
	links map[uuid.UUID]string
}

func New() *Store {
	return &Store{links: make(map[uuid.UUID]string)}
}

func (s *Store) GetOrCreate(bookingID uuid.UUID) string {
	s.mu.RLock()
	if link, ok := s.links[bookingID]; ok {
		s.mu.RUnlock()
		return link
	}
	s.mu.RUnlock()

	s.mu.Lock()
	defer s.mu.Unlock()

	if link, ok := s.links[bookingID]; ok {
		return link
	}

	link := fmt.Sprintf("https://meet.example.com/%s", bookingID.String()[:8])
	s.links[bookingID] = link
	return link
}
