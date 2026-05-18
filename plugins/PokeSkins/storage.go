package pokeskins

import (
	"encoding/json"
	"os"
	"sync"
	"time"
)

// SkinPreference stores what skin a player wants (no texture data).
type SkinPreference struct {
	Type      string    `json:"type"`      // "premium" or "url"
	Target    string    `json:"target"`    // username (premium) or image URL (url)
	CreatedAt time.Time `json:"createdAt"` // for TTL cleanup
}

// SkinStorage manages player skin preferences.
type SkinStorage struct {
	mu       sync.RWMutex
	prefs    map[string]SkinPreference // key = playerUUID (undashed)
	filePath string
	ttl      time.Duration // calculated from config (days)
	stopChan chan struct{}
}

// NewSkinStorage creates a storage, loads existing file, and starts a cleanup goroutine.
func NewSkinStorage(filePath string, ttlDays int) (*SkinStorage, error) {
	s := &SkinStorage{
		prefs:    make(map[string]SkinPreference),
		filePath: filePath,
		ttl:      time.Duration(ttlDays) * 24 * time.Hour,
		stopChan: make(chan struct{}),
	}
	// load existing data
	if err := s.load(); err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	// start background cleanup every hour
	go s.cleanupLoop()
	return s, nil
}

// Stop stops the background cleanup goroutine.
func (s *SkinStorage) Stop() {
	close(s.stopChan)
}

// Set stores a skin preference for a player (overwrites existing).
func (s *SkinStorage) Set(playerUUID string, pref SkinPreference) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	pref.CreatedAt = time.Now()
	s.prefs[playerUUID] = pref
	return s.save()
}

// Get retrieves a skin preference. Returns (pref, true) if exists.
func (s *SkinStorage) Get(playerUUID string) (SkinPreference, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	p, ok := s.prefs[playerUUID]
	return p, ok
}

// Delete removes a player's preference.
func (s *SkinStorage) Delete(playerUUID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.prefs, playerUUID)
	return s.save()
}

// Clear removes all preferences (for testing or admin command).
func (s *SkinStorage) Clear() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.prefs = make(map[string]SkinPreference)
	return s.save()
}

// Count returns number of stored preferences.
func (s *SkinStorage) Count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.prefs)
}

// load reads JSON file into memory.
func (s *SkinStorage) load() error {
	data, err := os.ReadFile(s.filePath)
	if err != nil {
		return err
	}
	var prefs map[string]SkinPreference
	if err := json.Unmarshal(data, &prefs); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.prefs = prefs
	return nil
}

// save writes current preferences to JSON file.
// FIXED DEADLOCK: Caller (Set/Delete/Clear) already holds the main Lock.
// Do NOT call s.mu.RLock() or s.mu.Lock() inside here to prevent proxy freezing.
func (s *SkinStorage) save() error {
	data, err := json.MarshalIndent(s.prefs, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.filePath, data, 0644)
}

// cleanupExpired removes entries older than TTL.
func (s *SkinStorage) cleanupExpired() {
	now := time.Now()
	s.mu.Lock()
	defer s.mu.Unlock()
	toDelete := []string{}
	for uuid, pref := range s.prefs {
		if now.Sub(pref.CreatedAt) > s.ttl {
			toDelete = append(toDelete, uuid)
		}
	}
	if len(toDelete) > 0 {
		for _, uuid := range toDelete {
			delete(s.prefs, uuid)
		}
		_ = s.save()
	}
}

// cleanupLoop runs cleanup every hour until stopped.
func (s *SkinStorage) cleanupLoop() {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			s.cleanupExpired()
		case <-s.stopChan:
			return
		}
	}
}