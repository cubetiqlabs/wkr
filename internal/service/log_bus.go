package service

import (
	"sync"

	"github.com/cubetiqlabs/wkr/internal/model"
	"github.com/google/uuid"
)

// LogBus is an in-memory pub/sub for real-time invocation log streaming.
type LogBus struct {
	mu   sync.RWMutex
	subs map[uuid.UUID][]chan *model.Invocation // workerID -> channels
}

func NewLogBus() *LogBus {
	return &LogBus{subs: make(map[uuid.UUID][]chan *model.Invocation)}
}

func (b *LogBus) Subscribe(workerID uuid.UUID) chan *model.Invocation {
	ch := make(chan *model.Invocation, 32)
	b.mu.Lock()
	b.subs[workerID] = append(b.subs[workerID], ch)
	b.mu.Unlock()
	return ch
}

func (b *LogBus) Unsubscribe(workerID uuid.UUID, ch chan *model.Invocation) {
	b.mu.Lock()
	defer b.mu.Unlock()
	subs := b.subs[workerID]
	for i, s := range subs {
		if s == ch {
			b.subs[workerID] = append(subs[:i], subs[i+1:]...)
			close(ch)
			return
		}
	}
}

func (b *LogBus) HasSubscribers(workerID uuid.UUID) bool {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return len(b.subs[workerID]) > 0
}

func (b *LogBus) Publish(inv *model.Invocation) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	for _, ch := range b.subs[inv.WorkerID] {
		select {
		case ch <- inv:
		default: // drop if subscriber is slow
		}
	}
}
