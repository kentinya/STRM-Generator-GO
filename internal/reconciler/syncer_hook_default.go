//go:build !strmgen_testhooks

package reconciler

// afterOwnershipRecord is an inert production hook point. The optional
// test build can stop immediately after a durable ownership insert; normal
// binaries have no environment-driven control flow.
func afterOwnershipRecord(_ string) {}
