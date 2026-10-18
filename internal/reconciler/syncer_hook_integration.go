//go:build strmgen_testhooks

package reconciler

import (
	"os"
	"syscall"
)

// afterOwnershipRecord is a deterministic test-only barrier. When the
// environment selects this exact relative path, the process records an
// observable marker and then stops itself with SIGSTOP. It therefore pauses
// at exactly one reproducible window: the ownership row is durable, the next
// target has not been published and the round has not been committed. A
// release build contains only the inert hook and no such environment switch.
func afterOwnershipRecord(relativePath string) {
	want := os.Getenv("STRMGEN_TEST_OWNERSHIP_HOLD")
	if want == "" || want != relativePath {
		return
	}
	if marker := os.Getenv("STRMGEN_TEST_OWNERSHIP_MARKER"); marker != "" {
		_ = os.WriteFile(marker, []byte(relativePath), 0o600)
	}
	_ = syscall.Kill(syscall.Getpid(), syscall.SIGSTOP)
}
