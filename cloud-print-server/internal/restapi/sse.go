package restapi

import (
	"net/http"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/cloud-print/server/internal/domain"
)

type Event struct {
	Type    string      `json:"type"`
	TaskID  string      `json:"task_id,omitempty"`
	Status  string      `json:"status,omitempty"`
	Data    interface{} `json:"data,omitempty"`
	At      time.Time   `json:"at"`
}

type subscriber struct {
	ch     chan Event
	closed chan struct{}
}

type SSEHub struct {
	mu          sync.RWMutex
	subscribers map[string]map[*subscriber]struct{}
	logger      *zap.Logger
	bufSize     int
}

func NewSSEHub(logger *zap.Logger) *SSEHub {
	return &SSEHub{
		subscribers: make(map[string]map[*subscriber]struct{}),
		logger:      logger,
		bufSize:     32,
	}
}

func (h *SSEHub) Subscribe(userID string) (<-chan Event, func()) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.subscribers[userID] == nil {
		h.subscribers[userID] = make(map[*subscriber]struct{})
	}
	sub := &subscriber{
		ch:     make(chan Event, h.bufSize),
		closed: make(chan struct{}),
	}
	h.subscribers[userID][sub] = struct{}{}

	unsubscribe := func() {
		h.mu.Lock()
		defer h.mu.Unlock()
		if subs, ok := h.subscribers[userID]; ok {
			if _, ok := subs[sub]; ok {
				delete(subs, sub)
				close(sub.closed)
				close(sub.ch)
			}
			if len(subs) == 0 {
				delete(h.subscribers, userID)
			}
		}
	}
	return sub.ch, unsubscribe
}

func (h *SSEHub) Publish(userID string, event Event) {
	h.mu.RLock()
	subs := h.subscribers[userID]
	h.mu.RUnlock()

	if len(subs) == 0 {
		return
	}
	if event.At.IsZero() {
		event.At = time.Now().UTC()
	}
	for sub := range subs {
		select {
		case <-sub.closed:
			continue
		case sub.ch <- event:
		default:
			h.logger.Warn("sse subscriber buffer full, dropping event",
				zap.String("user_id", userID),
				zap.String("type", event.Type),
			)
		}
	}
}

func (h *SSEHub) NotifyTaskStatus(userID, taskID string, status domain.TaskStatus, payload interface{}) {
	if userID == "" {
		return
	}
	h.Publish(userID, Event{
		Type:   "task_status",
		TaskID: taskID,
		Status: status.String(),
		Data:   payload,
	})
}

func (h *SSEHub) SubscriberCount(userID string) int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.subscribers[userID])
}

func (h *SSEHub) TotalSubscribers() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	total := 0
	for _, subs := range h.subscribers {
		total += len(subs)
	}
	return total
}

func (h *SSEHub) ServeSSE(w http.ResponseWriter, r *http.Request, userID string) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	events, unsubscribe := h.Subscribe(userID)
	defer unsubscribe()

	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case ev, ok := <-events:
			if !ok {
				return
			}
			if err := writeSSEEvent(w, ev); err != nil {
				return
			}
			flusher.Flush()
		case <-ticker.C:
			if _, err := w.Write([]byte(": ping\n\n")); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}