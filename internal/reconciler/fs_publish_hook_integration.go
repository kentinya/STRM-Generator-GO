//go:build strmgen_testhooks

package reconciler

import (
	"errors"
	"os"

	"strm-generator-go/internal/securepath"
)

// This file exists only in binaries explicitly built with the
// strmgen_testhooks tag. It can create one external target after LocalFS's
// initial no-follow check and immediately before Linkat's no-overwrite
// publication. The release build contains only the inert hook.
func beforePublishLink(dir *securepath.Dir, target string) error {
	if os.Getenv("STRMGEN_TEST_PUBLISH_BEFORE_LINK_TARGET") != target {
		return nil
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
