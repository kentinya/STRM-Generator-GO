//go:build strmgen_testhooks

package reconciler

import (
	"os"
	"syscall"
)

// afterSubtitlePublish is a deterministic test-only barrier. When the
// environment selects this exact relative path, the process records an
// observable marker and then stops itself with SIGSTOP. It therefore pauses at
// exactly one reproducible window: the subtitle file is published and durable,
// but the ownership row is not registered and the round is not committed. A
// SIGKILL in this window leaves the unowned-file state an abrupt process death
// would leave, so the recovery rules can be verified without a timing race.
func afterSubtitlePublish(relativePath string) {
	want := os.Getenv("STRMGEN_TEST_SUBTITLE_HOLD")
	if want == "" || want != relativePath {
		return
	}
	if marker := os.Getenv("STRMGEN_TEST_SUBTITLE_MARKER"); marker != "" {
		_ = os.WriteFile(marker, []byte(relativePath), 0o600)
	}
	_ = syscall.Kill(syscall.Getpid(), syscall.SIGSTOP)
}
