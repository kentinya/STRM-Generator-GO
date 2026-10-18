//go:build strmgen_testhooks

package main

import (
	"os"
	"strconv"
	"time"
)

// serveIntervalOverride shortens the serve interval for the isolated
// scheduling integration driver only. This file exists exclusively in binaries
// built with the strmgen_testhooks tag; the release build contains only the
// inert default and no environment-driven scheduling switch. The override only
// changes timing: it never bypasses configuration validation, the shared lock,
// the output/DB boundary or the round commit rules.
func serveIntervalOverride(configured time.Duration) time.Duration {
	raw := os.Getenv("STRMGEN_TEST_SERVE_INTERVAL_MS")
	if raw == "" {
		return configured
	}
	ms, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || ms < 1 {
		return configured
	}
	return time.Duration(ms) * time.Millisecond
}
