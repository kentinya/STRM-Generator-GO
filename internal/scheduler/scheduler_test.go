package scheduler

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

// fakeClock is a deterministic clock. Tests advance it explicitly and it fires
// the timers whose deadline has been reached. It never uses real time.
type fakeClock struct {
	mu     sync.Mutex
	now    time.Time
	timers []*fakeTimer
}

type fakeTimer struct {
	deadline time.Time
	ch       chan time.Time
	stopped  bool
}

func newFakeClock() *fakeClock {
	return &fakeClock{now: time.Unix(0, 0)}
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) NewTimer(d time.Duration) Timer {
	c.mu.Lock()
	defer c.mu.Unlock()
	t := &fakeTimer{deadline: c.now.Add(d), ch: make(chan time.Time, 1)}
	c.timers = append(c.timers, t)
	return t
}

// pending reports how many timers are registered and not stopped.
func (c *fakeClock) pending() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	n := 0
	for _, t := range c.timers {
		if !t.stopped {
			n++
		}
	}
	return n
}

// advance moves the clock forward and fires every due timer once.
func (c *fakeClock) advance(d time.Duration) {
	c.mu.Lock()
	c.now = c.now.Add(d)
	now := c.now
	var due []*fakeTimer
	remaining := c.timers[:0]
	for _, t := range c.timers {
		if !t.stopped && !t.deadline.After(now) {
			due = append(due, t)
			continue
		}
		remaining = append(remaining, t)
	}
	c.timers = remaining
	c.mu.Unlock()
	for _, t := range due {
		t.ch <- now
	}
}

func (t *fakeTimer) C() <-chan time.Time { return t.ch }

func (t *fakeTimer) Stop() {
	t.stopped = true
}

// waitForTimer waits until the scheduler has registered its next timer.
func waitForTimer(t *testing.T, clock *fakeClock) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if clock.pending() > 0 {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("scheduler did not register a timer")
}

func TestImmediateFirstRoundThenFixedInterval(t *testing.T) {
	clock := newFakeClock()
	var mu sync.Mutex
	var rounds []time.Time
	round := func(_ context.Context, _ bool) Result {
		mu.Lock()
		rounds = append(rounds, clock.Now())
		mu.Unlock()
		return Result{}
	}
	var events []Event
	sched := New(5*time.Minute, round, clock, func(e Event) {
		mu.Lock()
		events = append(events, e)
		mu.Unlock()
	})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan Event, 1)
	go func() { done <- sched.Run(ctx) }()

	// The first round runs immediately, before any clock advance.
	deadline := time.Now().Add(2 * time.Second)
	for {
		mu.Lock()
		n := len(rounds)
		mu.Unlock()
		if n >= 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("first round did not run immediately")
		}
		time.Sleep(time.Millisecond)
	}
	mu.Lock()
	if !rounds[0].Equal(time.Unix(0, 0)) {
		t.Fatalf("first round ran at %v, want the initial clock instant", rounds[0])
	}
	mu.Unlock()

	waitForTimer(t, clock)
	clock.advance(5 * time.Minute)
	deadline = time.Now().Add(2 * time.Second)
	for {
		mu.Lock()
		n := len(rounds)
		mu.Unlock()
		if n >= 2 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("second round did not run after one interval")
		}
		time.Sleep(time.Millisecond)
	}
	mu.Lock()
	if got := rounds[1].Sub(rounds[0]); got != 5*time.Minute {
		t.Fatalf("second round delta = %v, want 5m", got)
	}
	mu.Unlock()

	cancel()
	select {
	case event := <-done:
		if event.Kind != Cancelled {
			t.Fatalf("terminal kind = %s, want cancelled", event.Kind)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("scheduler did not stop after cancel")
	}
}

func TestBusyAndFailureContinueScheduling(t *testing.T) {
	clock := newFakeClock()
	results := []Result{{Busy: true}, {Err: errors.New("api down")}, {}}
	var mu sync.Mutex
	index := 0
	round := func(_ context.Context, _ bool) Result {
		mu.Lock()
		defer mu.Unlock()
		r := results[index]
		if index < len(results)-1 {
			index++
		}
		return r
	}
	var kinds []Kind
	sched := New(time.Minute, round, clock, func(e Event) {
		mu.Lock()
		kinds = append(kinds, e.Kind)
		mu.Unlock()
	})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan Event, 1)
	go func() { done <- sched.Run(ctx) }()

	for i := 0; i < 3; i++ {
		waitForTimer(t, clock)
		clock.advance(time.Minute)
	}
	// Wait for the third classification.
	deadline := time.Now().Add(2 * time.Second)
	for {
		mu.Lock()
		n := len(kinds)
		mu.Unlock()
		if n >= 3 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("scheduler did not classify three rounds")
		}
		time.Sleep(time.Millisecond)
	}
	mu.Lock()
	want := []Kind{BusySkip, Failure, Success}
	for i, k := range want {
		if kinds[i] != k {
			t.Fatalf("event %d = %s, want %s", i, kinds[i], k)
		}
	}
	mu.Unlock()
	cancel()
	<-done
}

func TestSlowRoundDoesNotBacklog(t *testing.T) {
	clock := newFakeClock()
	var mu sync.Mutex
	var starts []time.Time
	release := make(chan struct{})
	first := true
	round := func(_ context.Context, _ bool) Result {
		mu.Lock()
		starts = append(starts, clock.Now())
		mu.Unlock()
		if first {
			first = false
			<-release
		}
		return Result{}
	}
	sched := New(time.Minute, round, clock, func(Event) {})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan Event, 1)
	go func() { done <- sched.Run(ctx) }()

	// Wait for the first round to be in flight, then advance far beyond one
	// interval. Because the scheduler only registers the next timer after the
	// round returns, no backlog of ticks can exist.
	deadline := time.Now().Add(2 * time.Second)
	for {
		mu.Lock()
		n := len(starts)
		mu.Unlock()
		if n >= 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("first round did not start")
		}
		time.Sleep(time.Millisecond)
	}
	clock.advance(10 * time.Minute)
	if clock.pending() != 0 {
		t.Fatalf("scheduler queued %d timers while a round was running", clock.pending())
	}
	close(release)
	waitForTimer(t, clock)
	mu.Lock()
	if len(starts) != 1 {
		t.Fatalf("slow round produced %d extra rounds before the next tick", len(starts)-1)
	}
	mu.Unlock()
	cancel()
	<-done
}

func TestFatalStopsScheduler(t *testing.T) {
	clock := newFakeClock()
	calls := 0
	round := func(_ context.Context, _ bool) Result {
		calls++
		return Result{Fatal: true, Err: errors.New("lock unavailable")}
	}
	sched := New(time.Minute, round, clock, func(Event) {})
	event := sched.Run(context.Background())
	if event.Kind != Fatal {
		t.Fatalf("kind = %s, want fatal", event.Kind)
	}
	if calls != 1 {
		t.Fatalf("fatal round called %d times, want 1", calls)
	}
	if clock.pending() != 0 {
		t.Fatalf("fatal scheduler left %d timers", clock.pending())
	}
}

func TestCancelledBetweenTicksDoesNotStartRound(t *testing.T) {
	clock := newFakeClock()
	calls := 0
	round := func(_ context.Context, _ bool) Result {
		calls++
		return Result{}
	}
	ctx, cancel := context.WithCancel(context.Background())
	var events []Event
	sched := New(time.Minute, round, clock, func(e Event) { events = append(events, e) })
	done := make(chan Event, 1)
	go func() { done <- sched.Run(ctx) }()

	waitForTimer(t, clock)
	cancel()
	select {
	case event := <-done:
		if event.Kind != Cancelled {
			t.Fatalf("kind = %s, want cancelled", event.Kind)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("scheduler did not stop after cancellation between ticks")
	}
	if calls != 1 {
		t.Fatalf("scheduler started %d rounds, want only the immediate first round", calls)
	}
}

func TestFirstFlagMarksOnlyImmediateRound(t *testing.T) {
	clock := newFakeClock()
	var flags []bool
	round := func(_ context.Context, first bool) Result {
		flags = append(flags, first)
		return Result{}
	}
	ctx, cancel := context.WithCancel(context.Background())
	sched := New(time.Minute, round, clock, func(Event) {})
	done := make(chan Event, 1)
	go func() { done <- sched.Run(ctx) }()
	waitForTimer(t, clock)
	clock.advance(time.Minute)
	deadline := time.Now().Add(2 * time.Second)
	for {
		if len(flags) >= 2 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("second round did not run")
		}
		time.Sleep(time.Millisecond)
	}
	cancel()
	<-done
	if !flags[0] || flags[1] {
		t.Fatalf("first flags = %v, want [true false]", flags)
	}
}

func TestKindStrings(t *testing.T) {
	cases := map[Kind]string{Success: "success", Failure: "failure", BusySkip: "busy-skip", Cancelled: "cancelled", Fatal: "fatal"}
	for kind, want := range cases {
		if got := kind.String(); got != want {
			t.Fatalf("Kind(%d).String() = %q, want %q", kind, got, want)
		}
	}
}
