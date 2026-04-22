package session

import (
	"crypto/rand"
	"encoding/base64"
	"sync"
	"time"
)

type Session struct {
	ID        string    `json:"-"`
	AdminID   int64     `json:"admin_id"`
	Username  string    `json:"username"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
}

type Store struct {
	mu       sync.RWMutex
	sessions map[string]*Session
	ttl      time.Duration
}

func NewStore(ttl time.Duration) *Store {
	if ttl <= 0 {
		ttl = 8 * time.Hour
	}
	return &Store{sessions: make(map[string]*Session), ttl: ttl}
}

func (s *Store) Create(adminID int64, username string) (*Session, error) {
	id, err := randomID()
	if err != nil {
		return nil, err
	}
	now := time.Now().In(time.Local)
	sess := &Session{ID: id, AdminID: adminID, Username: username, CreatedAt: now, ExpiresAt: now.Add(s.ttl)}
	s.mu.Lock()
	s.sessions[id] = sess
	s.mu.Unlock()
	return sess, nil
}

func (s *Store) Get(id string) (*Session, bool) {
	s.mu.RLock()
	sess, ok := s.sessions[id]
	s.mu.RUnlock()
	if !ok || time.Now().In(time.Local).After(sess.ExpiresAt) {
		if ok {
			s.Delete(id)
		}
		return nil, false
	}
	return sess, true
}

func (s *Store) Delete(id string) {
	s.mu.Lock()
	delete(s.sessions, id)
	s.mu.Unlock()
}

func (s *Store) Cleanup(stop <-chan struct{}) {
	ticker := time.NewTicker(10 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			s.cleanupOnce()
		case <-stop:
			return
		}
	}
}

func (s *Store) TTL() time.Duration {
	return s.ttl
}

func (s *Store) cleanupOnce() {
	now := time.Now().In(time.Local)
	s.mu.Lock()
	for id, sess := range s.sessions {
		if now.After(sess.ExpiresAt) {
			delete(s.sessions, id)
		}
	}
	s.mu.Unlock()
}

func randomID() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
