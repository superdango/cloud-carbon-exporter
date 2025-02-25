package must

import "time"

type Wait struct {
	max        time.Duration
	current    time.Duration
	occurences int
}

func NewWait(max time.Duration) *Wait {
	return &Wait{
		max:        max,
		current:    time.Duration(time.Millisecond),
		occurences: 0,
	}
}

func (w *Wait) Reset() {
	w.current = time.Duration(time.Millisecond)
}

func (w *Wait) Linearly(step time.Duration) {
	sleep := step * time.Duration(w.occurences)
	time.Sleep(min(sleep, w.max))
	w.occurences++
}
