//go:build strmgen_testhooks

package reconciler

import (
	"context"
	"errors"
	"os"
	"syscall"
	"time"

	"strm-generator-go/internal/securepath"
)

// This file exists only in binaries explicitly built with the
// strmgen_testhooks tag. It provides deterministic barriers and failure
// injection around the owned-target unlink window. The release build contains
// only the inert hooks and no environment switches.

// beforeDeleteUnlink pauses before an owned target is unlinked. A gate file is
// polled so the Go runtime keeps running and can react to context
// cancellation; a hold stops the process itself for SIGKILL tests.
func beforeDeleteUnlink(ctx context.Context, _ *securepath.Dir, target string) error {
	if gate := os.Getenv("STRMGEN_TEST_DELETE_BEFORE_UNLINK_GATE"); gate != "" {
		writeDeleteMarker("STRMGEN_TEST_DELETE_BEFORE_UNLINK_MARKER", target)
		for {
			if _, err := os.Stat(gate); err == nil {
				break
			}
			time.Sleep(2 * time.Millisecond)
		}
	}
	if os.Getenv("STRMGEN_TEST_DELETE_BEFORE_UNLINK_HOLD") == target {
		writeDeleteMarker("STRMGEN_TEST_DELETE_BEFORE_UNLINK_MARKER", target)
		_ = syscall.Kill(syscall.Getpid(), syscall.SIGSTOP)
	}
	return ctx.Err()
}

// afterDeleteUnlink runs between the unlink and the parent directory sync. It
// can fail the sync for one exact target or hold the process at the same
// window so a SIGKILL leaves the exact recoverable state.
func afterDeleteUnlink(ctx context.Context, _ *securepath.Dir, target string) error {
	if os.Getenv("STRMGEN_TEST_DELETE_AFTER_UNLINK_FAIL_TARGET") == target {
		return errors.New("test directory sync failure after unlink")
	}
	if gate := os.Getenv("STRMGEN_TEST_DELETE_AFTER_UNLINK_GATE"); gate != "" {
		writeDeleteMarker("STRMGEN_TEST_DELETE_AFTER_UNLINK_MARKER", target)
		for {
			if _, err := os.Stat(gate); err == nil {
				break
			}
			time.Sleep(2 * time.Millisecond)
		}
	}
	if os.Getenv("STRMGEN_TEST_DELETE_AFTER_UNLINK_HOLD") == target {
		writeDeleteMarker("STRMGEN_TEST_DELETE_AFTER_UNLINK_MARKER", target)
		_ = syscall.Kill(syscall.Getpid(), syscall.SIGSTOP)
	}
	return ctx.Err()
}

func writeDeleteMarker(env, target string) {
	if marker := os.Getenv(env); marker != "" {
		_ = os.WriteFile(marker, []byte(target), 0o600)
	}
}
