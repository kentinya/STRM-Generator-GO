//go:build strmgen_testhooks

package reconciler

import (
	"os"
	"strconv"

	"strm-generator-go/internal/securepath"
)

// This file exists only in binaries explicitly built with the
// strmgen_testhooks tag. It can truncate a staging write to a configured byte
// count and hold the process at the two interrupted-staging windows so a
// SIGKILL leaves exactly the state an abrupt process death would leave. The
// release build contains only the inert hooks and no environment switches.

func stagingWriteData(data []byte) []byte {
	raw := os.Getenv("STRMGEN_TEST_STAGING_PARTIAL_BYTES")
	if raw == "" {
		return data
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 0 || n > len(data) {
		return data
	}
	return data[:n]
}

func beforeStagingWrite(_ *securepath.Dir, target string) error {
	updateBarrier("STRMGEN_TEST_STAGING_BEFORE_WRITE_GATE",
		"STRMGEN_TEST_STAGING_BEFORE_WRITE_HOLD",
		"STRMGEN_TEST_STAGING_BEFORE_WRITE_MARKER", target)
	return nil
}

func afterStagingWrite(_ *securepath.Dir, target string) error {
	updateBarrier("STRMGEN_TEST_STAGING_PARTIAL_GATE",
		"STRMGEN_TEST_STAGING_PARTIAL_HOLD",
		"STRMGEN_TEST_STAGING_PARTIAL_MARKER", target)
	return nil
}
