package service

import (
	"context"
	"sync"
)

// operationLifetime owns the ordering shared by authenticated requests and app
// changes. A change cancels and drains the preceding requests before entering;
// its successor cannot cancel an obsolete lifetime while this one is finishing.
type operationLifetime struct {
	requests   sync.RWMutex
	changes    sync.Mutex
	mu         sync.Mutex
	generation context.Context
	cancel     context.CancelFunc
	stopped    bool
}

func newOperationLifetime() *operationLifetime {
	lifetime := &operationLifetime{}
	lifetime.generation, lifetime.cancel = context.WithCancel(context.Background())
	return lifetime
}

func (l *operationLifetime) read() (context.Context, func()) {
	l.requests.RLock()
	l.mu.Lock()
	generation := l.generation
	l.mu.Unlock()
	return generation, l.requests.RUnlock
}

// change optionally checks a queued action after preceding changes finish, but
// before canceling any current requests. This keeps superseded startup a no-op.
func (l *operationLifetime) change(needed func() bool) (func(), bool) {
	l.changes.Lock()
	if needed != nil && !needed() {
		l.changes.Unlock()
		return nil, false
	}
	l.mu.Lock()
	l.cancel()
	l.mu.Unlock()
	l.requests.Lock()
	var once sync.Once
	return func() {
		once.Do(func() {
			l.mu.Lock()
			if !l.stopped {
				l.generation, l.cancel = context.WithCancel(context.Background())
			}
			l.mu.Unlock()
			l.requests.Unlock()
			l.changes.Unlock()
		})
	}, true
}

func (l *operationLifetime) stop() func() {
	l.mu.Lock()
	l.stopped = true
	l.cancel()
	l.mu.Unlock()
	finish, _ := l.change(nil)
	return finish
}
