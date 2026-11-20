//go:build !strmgen_testhooks

package reconciler

// afterSubtitlePublish is an inert production hook point. The optional test
// build can stop immediately after one subtitle was durably published but
// before its ownership row was registered; normal binaries have no
// environment-driven control flow.
func afterSubtitlePublish(_ string) {}
