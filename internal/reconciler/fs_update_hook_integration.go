//go:build strmgen_testhooks

package reconciler

import (
	"errors"
	"os"
	"syscall"
	"time"

	"strm-generator-go/internal/securepath"
)

// This file exists only in binaries explicitly built with the
// strmgen_testhooks tag. It can replace an owned target with external bytes
// after LocalFS's no-follow checks and immediately before the atomic exchange,
// or hold the process at the pre-exchange and post-exchange windows. The
// release build contains only the inert hooks and no environment switches.

func beforeUpdateExchange(dir *securepath.Dir, target string) error {
	if os.Getenv("STRMGEN_TEST_UPDATE_BEFORE_EXCHANGE_TARGET") == target {
		if err := dir.Remove(target); err != nil {
			return errors.New("test target removal failed")
		}
		f, err := dir.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if err != nil {
			return errors.New("test target creation failed")
		}
		if err := writeAll(f, []byte("external")); err != nil {
			_ = f.Close()
			return errors.New("test target write failed")
		}
		if err := f.Close(); err != nil {
			return errors.New("test target close failed")
		}
	}
	updateBarrier("STRMGEN_TEST_UPDATE_PRE_EXCHANGE_GATE",
		"STRMGEN_TEST_UPDATE_PRE_EXCHANGE_HOLD",
		"STRMGEN_TEST_UPDATE_PRE_EXCHANGE_MARKER", target)
	return nil
}

func afterUpdateExchange(_ *securepath.Dir, target string) error {
	updateBarrier("STRMGEN_TEST_UPDATE_EXCHANGE_GATE",
		"STRMGEN_TEST_UPDATE_EXCHANGE_HOLD",
		"STRMGEN_TEST_UPDATE_EXCHANGE_MARKER", target)
	return nil
}

// updateBarrier pauses at one deterministic update window. A gate file is
// polled so the Go runtime keeps running and can react to context cancellation
// (SIGTERM); a hold instead stops the process itself for SIGKILL tests.
func updateBarrier(gateEnv, holdEnv, markerEnv, target string) {
	if gate := os.Getenv(gateEnv); gate != "" {
		if marker := os.Getenv(markerEnv); marker != "" {
			_ = os.WriteFile(marker, []byte(target), 0o600)
		}
		for {
			if _, err := os.Stat(gate); err == nil {
				return
			}
			time.Sleep(2 * time.Millisecond)
		}
	}
	if os.Getenv(holdEnv) != target {
		return
	}
	if marker := os.Getenv(markerEnv); marker != "" {
		_ = os.WriteFile(marker, []byte(target), 0o600)
	}
	_ = syscall.Kill(syscall.Getpid(), syscall.SIGSTOP)
}
