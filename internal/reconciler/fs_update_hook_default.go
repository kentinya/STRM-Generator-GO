//go:build !strmgen_testhooks

package reconciler

import "strm-generator-go/internal/securepath"

// beforeUpdateExchange is an inert production hook point. The optional test
// build supplies a controlled race injector at the exact moment before an
// owned target is atomically exchanged; normal binaries have no
// environment-driven filesystem behavior.
func beforeUpdateExchange(_ *securepath.Dir, _ string) error { return nil }

// afterUpdateExchange is an inert production hook point. The optional test
// build can stop the process exactly after the atomic exchange and before the
// swapped-out content is verified, so the interrupted-verification window is
// reproducible without a timing race.
func afterUpdateExchange(_ *securepath.Dir, _ string) error { return nil }
