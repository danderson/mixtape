package timing

import (
	"bytes"
	"fmt"
	"sync"
	"time"
)

type Phase struct {
	Name     string
	Duration time.Duration
}

type Rec struct {
	mu     sync.Mutex
	start  time.Time
	phases []Phase
}

func (r *Rec) finishLocked() {
	if !r.start.IsZero() {
		r.phases[len(r.phases)-1].Duration = time.Since(r.start)
	}
}

func (r *Rec) Phase(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.finishLocked()
	r.start = time.Now()
	r.phases = append(r.phases, Phase{Name: name})
}

func (r *Rec) Done() Timings {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.finishLocked()
	r.start = time.Time{}
	ph := r.phases
	r.phases = nil
	return ph
}

type Timings []Phase

func (t Timings) Total() time.Duration {
	var ret time.Duration
	for _, p := range t {
		ret += p.Duration
	}
	return ret
}

func (t Timings) DebugString() string {
	var b bytes.Buffer
	for _, p := range t {
		fmt.Fprintf(&b, "%s %v\n", p.Name, p.Duration)
	}
	fmt.Fprintf(&b, "total %v\n", t.Total())
	return b.String()
}
