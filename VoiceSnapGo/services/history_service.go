package services

import (
	"voicesnap/internal/history"
)

// HistoryService provides recognition history to the frontend.
type HistoryService struct {
	store *history.Store
}

func NewHistoryService(store *history.Store) *HistoryService {
	return &HistoryService{store: store}
}

// GetAll returns all history entries (newest first).
func (s *HistoryService) GetAll() []history.Entry {
	return s.store.GetAll()
}

// Add adds a new recognition result to history.
func (s *HistoryService) Add(text string) {
	s.store.Add(text)
}

// Delete removes a single entry by timestamp.
func (s *HistoryService) Delete(timestamp int64) {
	s.store.Delete(timestamp)
}

// ClearAll removes all history entries.
func (s *HistoryService) ClearAll() {
	s.store.ClearAll()
}

// GetRetentionDays returns the current retention period in days.
func (s *HistoryService) GetRetentionDays() int {
	return s.store.GetRetentionDays()
}

// SetRetentionDays sets how long to keep history (0 = forever).
func (s *HistoryService) SetRetentionDays(days int) {
	s.store.SetRetentionDays(days)
}
