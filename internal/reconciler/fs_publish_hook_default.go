//go:build !strmgen_testhooks

package reconciler

import "strm-generator-go/internal/securepath"

// beforePublishLink is an inert production hook point. The optional test build
// supplies a controlled race injector; normal binaries have no environment-
// driven filesystem behavior.
func beforePublishLink(_ *securepath.Dir, _ string) error { return nil }
