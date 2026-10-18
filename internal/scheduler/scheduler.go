// Package scheduler runs a bounded sequence of synchronization rounds for the
// long-running serve entry point. It owns no file, database or lock state and
// never runs two rounds at once: it only decides when to invoke a caller
// supplied round function and classifies each outcome.
//
// The design is intentionally minimal. There is exactly one timer and no
// goroutine per tick, so a slow round can never accumulate a backlog of queued
// ticks or overlap with the next round. Scheduling state is never persisted:
// a restart resumes from the durable ownership/round facts, not from an
// in-memory tick.
package scheduler

import (
	"context"
	"time"
)

// Kind classifies one scheduler event.
type Kind int

const (
	// Success is a round that completed without error.
	Success Kind = iota
	// Failure is a round that returned a runtime error. The scheduler keeps
	// running and retries on the next interval.
	Failure
	// BusySkip is a round that was skipped because the shared lock was held by
	// another run. It is never reported as a success round.
	BusySkip
	// Cancelled is a round or wait ended by context cancellation.
	Cancelled
	// Fatal is an unrecoverable startup or resource error. The scheduler stops.
	Fatal
)

// String renders the classification using the fixed contract vocabulary.
func (k Kind) String() string {
	switch k {
	case Success:
		return "success"
	case Failure:
		return "failure"
	case BusySkip:
		return "busy-skip"
	case Cancelled:
		return "cancelled"
	case Fatal:
		return "fatal"
	default:
		return "unknown"
	}
}

// Result is what one round invocation returns to the scheduler.
//
// Busy and Fatal are explicit because they are not ordinary runtime errors: a
// busy skip must not be counted as a success or a failure, and a fatal
// resource error stops the scheduler instead of retrying forever.
type Result struct {
	// Busy reports the shared lock was held by another run.
	Busy bool
	// Fatal reports an unrecoverable startup/resource error.
	Fatal bool
	// Err is the (already redacted) failure reason.
	Err error
}

// Event describes one completed round or the terminal condition.
type Event struct {
	Kind Kind
	Err  error
	// Next is the scheduled time of the following round; zero when the
	// scheduler will not run another round.
	Next time.Time
}

// Round runs exactly one synchronization round. It must honor ctx and must
// never be invoked concurrently. first is true for the initial immediate round
// so the caller can treat startup resource errors as fatal while a later tick
// treats the same error as a retryable failure.
type Round func(ctx context.Context, first bool) Result

// Clock abstracts the timer used between rounds so tests can advance time
// deterministically. The production clock uses time.Now and time.NewTimer.
type Clock interface {
	Now() time.Time
	NewTimer(d time.Duration) Timer
}

// Timer abstracts one timer.
type Timer interface {
	C() <-chan time.Time
	Stop()
}

// Scheduler runs rounds on a fixed interval.
type Scheduler struct {
	interval time.Duration
	round    Round
	clock    Clock
	onEvent  func(Event)
}

// New builds a Scheduler. A nil clock uses the real clock; a nil onEvent is
// allowed and discards events.
func New(interval time.Duration, round Round, clock Clock, onEvent func(Event)) *Scheduler {
	if clock == nil {
		clock = realClock{}
	}
	if onEvent == nil {
		onEvent = func(Event) {}
	}
	return &Scheduler{interval: interval, round: round, clock: clock, onEvent: onEvent}
}

// Run executes the immediate first round and then waits on a single reusable
// timer for each subsequent round. It returns the terminal event and never
// creates a goroutine per tick, never queues a backlog and never overlaps
// rounds. A completed round is reported exactly once.
func (s *Scheduler) Run(ctx context.Context) Event {
	first := true
	for {
		if err := ctx.Err(); err != nil {
			// A signal between ticks must not start another round.
			return s.emit(Event{Kind: Cancelled, Err: err})
		}
		result := s.round(ctx, first)
		first = false
		event := classify(ctx, result)
		switch event.Kind {
		case Fatal, Cancelled:
			return s.emit(event)
		}
		// Schedule the next round on a single timer. The interval is measured
		// from the end of this round, so a slow round is followed by a full
		// interval and cannot create a catch-up backlog.
		event.Next = s.clock.Now().Add(s.interval)
		s.onEvent(event)
		if !s.wait(ctx) {
			return s.emit(Event{Kind: Cancelled, Err: ctx.Err()})
		}
	}
}

func (s *Scheduler) emit(event Event) Event {
	s.onEvent(event)
	return event
}

// classify turns one round result into an event. A cancelled context always
// wins over the round result so a cancelled round can never be recorded as a
// success or an ordinary failure.
func classify(ctx context.Context, result Result) Event {
	switch {
	case ctx.Err() != nil:
		return Event{Kind: Cancelled, Err: ctx.Err()}
	case result.Fatal:
		return Event{Kind: Fatal, Err: result.Err}
	case result.Busy:
		return Event{Kind: BusySkip}
	case result.Err != nil:
		return Event{Kind: Failure, Err: result.Err}
	default:
		return Event{Kind: Success}
	}
}

// wait blocks until the interval elapses or ctx is cancelled. It returns true
// when the timer fired and false when ctx ended. Exactly one timer exists at a
// time, so no stale tick can accumulate.
func (s *Scheduler) wait(ctx context.Context) bool {
	timer := s.clock.NewTimer(s.interval)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C():
		return true
	}
}

type realClock struct{}

func (realClock) Now() time.Time { return time.Now() }

func (realClock) NewTimer(d time.Duration) Timer { return realTimer{timer: time.NewTimer(d)} }

type realTimer struct{ timer *time.Timer }

func (t realTimer) C() <-chan time.Time { return t.timer.C }

func (t realTimer) Stop() { t.timer.Stop() }
