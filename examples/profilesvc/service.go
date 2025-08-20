package profilesvc

import (
	"context"
	"errors"
	"sync"
)

// Service is a simple CRUD interface for user profiles.
type Service interface {
	PostProfile(ctx context.Context, p Profile) error
	GetProfile(ctx context.Context, id string) (Profile, error)
	PutProfile(ctx context.Context, id string, p Profile) error
}

// Profile represents a single user profile.
// ID should be globally unique.
type Profile struct {
	ID        string    `json:"id"`
	Name      string    `json:"name,omitempty"`
	Addresses []Address `json:"addresses,omitempty"`
}

// Address is a field of a user profile.
// ID should be unique within the profile (at a minimum).
type Address struct {
	ID       string `json:"id"`
	Location string `json:"location,omitempty"`
}

var (
	ErrInconsistentIDs = errors.New("inconsistent IDs")
	ErrAlreadyExists   = errors.New("already exists")
	ErrNotFound        = errors.New("not found")
)

type inmemService struct {
	mtx sync.RWMutex
	m   map[string]Profile
}

func NewInmemService() Service {
	return &inmemService{
		m: map[string]Profile{},
	}
}

func (s *inmemService) PostProfile(ctx context.Context, p Profile) error {
	s.mtx.Lock()
	defer s.mtx.Unlock()
	if _, ok := s.m[p.ID]; ok {
		return ErrAlreadyExists // POST = create, don't overwrite
	}
	s.m[p.ID] = p
	return nil
}

func (s *inmemService) GetProfile(ctx context.Context, id string) (Profile, error) {
	s.mtx.RLock()
	defer s.mtx.RUnlock()
	p, ok := s.m[id]
	if !ok {
		return Profile{}, ErrNotFound
	}
	return p, nil
}

func (s *inmemService) PutProfile(ctx context.Context, id string, p Profile) error {
	if id != p.ID {
		return ErrInconsistentIDs
	}
	s.mtx.Lock()
	defer s.mtx.Unlock()
	s.m[id] = p // PUT = create or update
	return nil
}
