//go:build !strmgen_testhooks

package reconciler

import (
	"context"

	"strm-generator-go/internal/securepath"
)

// beforeDeleteUnlink is an inert production hook point. The optional test
// build can pause or stop the process exactly before an owned target is
// unlinked; normal binaries have no environment-driven control flow.
func beforeDeleteUnlink(_ context.Context, _ *securepath.Dir, _ string) error { return nil }

// afterDeleteUnlink is an inert production hook point. The optional test build
// can fail the directory sync or stop the process between the unlink and the
// ownership removal so the recovery window is deterministic.
func afterDeleteUnlink(_ context.Context, _ *securepath.Dir, _ string) error { return nil }
