//go:build !strmgen_testhooks

package main

// telegramAPIBase returns the official Bot API origin in release builds. The
// release binary has no environment-driven endpoint override.
func telegramAPIBase() string {
	return "https://api.telegram.org"
}
