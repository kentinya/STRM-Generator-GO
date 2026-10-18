//go:build !strmgen_testhooks

package main

import "time"

// serveIntervalOverride returns the configured interval unchanged in a release
// build. The release binary has no environment-driven scheduling override: the
// serve interval always comes from the validated configuration.
func serveIntervalOverride(configured time.Duration) time.Duration {
	return configured
}
