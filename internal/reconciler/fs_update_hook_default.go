//go:build !strmgen_testhooks

package reconciler

import "strm-generator-go/internal/securepath"

// beforeUpdateExchange is an inert production hook point. The optional test
// build supplies a controlled race injector at the exact moment before an
// owned target is atomically exchanged; normal binaries have no
// environment-driven filesystem behavior.
func beforeUpdateExchange(_ *securepath.Dir, _ string) error { return nil }
