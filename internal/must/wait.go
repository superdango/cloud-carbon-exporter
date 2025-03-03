package must

import (
	"sync"
	"time"
)

type Wait struct {
	mu         *sync.Mutex
	max        time.Duration
	current    time.Duration
	occurences int
}

func NewWait(max time.Duration) *Wait {
	return &Wait{
		mu:         new(sync.Mutex),
		max:        max,
		current:    time.Duration(time.Millisecond),
		occurences: 0,
	}
}

func (w *Wait) Reset() {
	w.mu.Lock()
	defer w.mu.Unlock()

	w.current = time.Duration(time.Millisecond)
}

func (w *Wait) Static(d time.Duration) {
	time.Sleep(d)
}

func (w *Wait) Linearly(step time.Duration) {
	sleep := step * time.Duration(w.occurences)
	time.Sleep(min(sleep, w.max))

	w.mu.Lock()
	defer w.mu.Unlock()
	w.occurences++
}
