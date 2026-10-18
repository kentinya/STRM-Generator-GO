//go:build strmgen_testhooks

package reconciler

import (
	"os"
	"syscall"

	"strm-generator-go/internal/securepath"
)

// afterUpdatePublish is a deterministic test-only barrier. When the
// environment selects this exact relative path, the process records an
// observable marker and stops itself with SIGSTOP. It therefore pauses at
// exactly one reproducible window: an owned target was atomically replaced,
// ownership still maps to it, but the complete round has not been committed.
// A release build contains only the inert hook.
func afterUpdatePublish(dir *securepath.Dir, target string) error {
	want := os.Getenv("STRMGEN_TEST_UPDATE_HOLD")
	if want == "" || want != target {
		return nil
	}
	if marker := os.Getenv("STRMGEN_TEST_UPDATE_MARKER"); marker != "" {
		_ = os.WriteFile(marker, []byte(target), 0o600)
	}
	_ = syscall.Kill(syscall.Getpid(), syscall.SIGSTOP)
	return nil
}
