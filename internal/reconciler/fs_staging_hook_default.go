//go:build !strmgen_testhooks

package reconciler

import "strm-generator-go/internal/securepath"

// stagingWriteData is the production policy: the complete document is written
// to the staging file. The optional test build can truncate it to reproduce an
// interrupted partial write deterministically.
func stagingWriteData(data []byte) []byte { return data }

// beforeStagingWrite is an inert production hook point. The optional test
// build can stop the process exactly after the deterministic staging file was
// created but before any byte is written.
func beforeStagingWrite(_ *securepath.Dir, _ string) error { return nil }

// afterStagingWrite is an inert production hook point. The optional test build
// can stop the process exactly after the (possibly truncated) staging write
// and before it is synced, closed and published.
func afterStagingWrite(_ *securepath.Dir, _ string) error { return nil }
