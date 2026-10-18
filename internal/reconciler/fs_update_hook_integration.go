//go:build strmgen_testhooks

package reconciler

import (
	"errors"
	"os"

	"strm-generator-go/internal/securepath"
)

// This file exists only in binaries explicitly built with the
// strmgen_testhooks tag. It can replace an owned target with external bytes
// after LocalFS's no-follow checks and immediately before the atomic exchange,
// exercising the post-exchange verification that preserves foreign content.
// The release build contains only the inert hook.
func beforeUpdateExchange(dir *securepath.Dir, target string) error {
	if os.Getenv("STRMGEN_TEST_UPDATE_BEFORE_EXCHANGE_TARGET") != target {
		return nil
	}
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
	return nil
}
