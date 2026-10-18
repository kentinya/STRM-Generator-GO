//go:build strmgen_testhooks

package reconciler

import "strm-generator-go/internal/securepath"

// afterUpdatePublish is a deterministic test-only barrier. When the
// environment selects this exact relative path, the process pauses after an
// owned target was atomically replaced and the old content removed, but before
// the complete round is committed. The release build contains only the inert
// hook. Gates poll a sentinel file so SIGTERM context cancellation can run;
// holds stop the process itself for SIGKILL tests.
func afterUpdatePublish(_ *securepath.Dir, target string) error {
	updateBarrier("STRMGEN_TEST_UPDATE_PUBLISH_GATE",
		"STRMGEN_TEST_UPDATE_HOLD",
		"STRMGEN_TEST_UPDATE_MARKER", target)
	return nil
}
