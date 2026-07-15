package inspects

import (
	"strings"
	"sync"
)

// inspectStore retains a bounded, newest-first history inside the Lighthouse process.
type inspectStore struct {
	mu       sync.RWMutex
	maxTotal int
	records  map[string]*inspectStoreEntry
	newest   *inspectStoreEntry
	oldest   *inspectStoreEntry
}

// inspectStoreEntry links a retained record into the newest-first history.
type inspectStoreEntry struct {
	inspectID string
	record    Record
	previous  *inspectStoreEntry
	next      *inspectStoreEntry
}

// newInspectStore creates a store whose memory use is bounded by maxTotal records.
func newInspectStore(maxTotal int) *inspectStore {
	if maxTotal <= 0 {
		maxTotal = defaultInspectMaxTotal
	}
	initialCapacity := minInt(maxTotal, defaultInspectMaxTotal)
	return &inspectStore{
		maxTotal: maxTotal,
		records:  make(map[string]*inspectStoreEntry, initialCapacity),
	}
}

// upsert stores a detached record and makes it the newest result.
func (s *inspectStore) upsert(record Record) {
	if s == nil {
		return
	}
	inspectID := strings.TrimSpace(record.Summary.TraceID)
	if inspectID == "" {
		return
	}
	record.Summary.TraceID = inspectID
	record = cloneRecord(record)

	s.mu.Lock()
	defer s.mu.Unlock()

	entry, exists := s.records[inspectID]
	if exists {
		entry.record = record
		s.detach(entry)
	} else {
		entry = &inspectStoreEntry{inspectID: inspectID, record: record}
		s.records[inspectID] = entry
	}
	s.prepend(entry)

	for len(s.records) > s.maxTotal {
		oldest := s.oldest
		if oldest == nil {
			break
		}
		s.detach(oldest)
		delete(s.records, oldest.inspectID)
	}
}

// recent returns detached summaries in newest-first order.
func (s *inspectStore) recent(query RecentQuery) []InspectSummary {
	if s == nil || query.Limit <= 0 {
		return []InspectSummary{}
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make([]InspectSummary, 0, minInt(query.Limit, len(s.records)))
	for entry := s.newest; entry != nil; entry = entry.next {
		if query.Source != "" && entry.record.Summary.Source != query.Source.String() {
			continue
		}
		out = append(out, cloneInspectSummary(entry.record.Summary))
		if len(out) == query.Limit {
			break
		}
	}
	return out
}

// byID returns a detached record so callers cannot mutate retained history.
func (s *inspectStore) byID(inspectID string) (Record, bool) {
	if s == nil {
		return Record{}, false
	}
	inspectID = strings.TrimSpace(inspectID)
	if inspectID == "" {
		return Record{}, false
	}

	s.mu.RLock()
	entry, ok := s.records[inspectID]
	if !ok {
		s.mu.RUnlock()
		return Record{}, false
	}
	record := cloneRecord(entry.record)
	s.mu.RUnlock()
	return record, true
}

// prepend makes entry the newest retained record.
func (s *inspectStore) prepend(entry *inspectStoreEntry) {
	entry.previous = nil
	entry.next = s.newest
	if s.newest != nil {
		s.newest.previous = entry
	} else {
		s.oldest = entry
	}
	s.newest = entry
}

// detach removes entry from the retention order while preserving its record.
func (s *inspectStore) detach(entry *inspectStoreEntry) {
	if entry.previous != nil {
		entry.previous.next = entry.next
	} else {
		s.newest = entry.next
	}
	if entry.next != nil {
		entry.next.previous = entry.previous
	} else {
		s.oldest = entry.previous
	}
	entry.previous = nil
	entry.next = nil
}

// cloneRecord detaches mutable fields from both writers and readers.
func cloneRecord(record Record) Record {
	record.Summary = cloneInspectSummary(record.Summary)
	if len(record.Events) == 0 {
		record.Events = nil
		return record
	}
	events := make([]InspectEvent, len(record.Events))
	for idx, event := range record.Events {
		event.Attributes = cloneAnyMap(event.Attributes)
		event.HTTP = cloneHTTPExchange(event.HTTP)
		events[idx] = event
	}
	record.Events = events
	return record
}

// cloneInspectSummary detaches summary labels from retained state.
func cloneInspectSummary(summary InspectSummary) InspectSummary {
	summary.Labels = cloneStringMap(summary.Labels)
	return summary
}
