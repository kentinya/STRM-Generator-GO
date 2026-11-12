//go:build strmgen_testhooks

package main

import (
	"os"

	"strm-generator-go/internal/notify"
)

// telegramAPIBase redirects the notification transport to a local test
// receiver in binaries built with the strmgen_testhooks tag only. This file
// does not exist in release builds; the release binary always uses the
// official API origin and TLS verification.
func telegramAPIBase() string {
	if base := os.Getenv("STRMGEN_TEST_TELEGRAM_API_BASE"); base != "" {
		return base
	}
	return notify.TelegramAPIBase
}
