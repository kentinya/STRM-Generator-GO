package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"strm-generator-go/internal/config"
	"strm-generator-go/internal/notify"
	"strm-generator-go/internal/reconciler"
	"strm-generator-go/internal/scheduler"
)

// notifier owns the process-local notification state. A disabled notifier
// never reads the notification environment variables, never creates a
// temporary listing and never sends anything.
type notifier struct {
	enabled   bool
	sender    *notify.Sender
	instance  string
	policy    string
	threshold int
	stderr    io.Writer
}

// setupNotifications applies the startup credential contract: a disabled
// notification configuration is inert, and an enabled configuration that is
// missing credentials only records a redacted warning and disables
// notifications for this process. dry-run never reads the credential
// environment variables.
func setupNotifications(resolved *config.Resolved, dryRun bool, stderr io.Writer) *notifier {
	n := &notifier{stderr: stderr, threshold: resolved.MissingConfirmations}
	if dryRun || !resolved.Notifications.Enabled {
		return n
	}
	tg := resolved.Notifications.Telegram
	token := os.Getenv(tg.BotTokenEnv)
	chatID := os.Getenv(tg.ChatIDEnv)
	if token == "" || chatID == "" {
		fmt.Fprintf(stderr,
			"notifications: telegram credential environment variables (%s, %s) are missing or empty; notifications are disabled for this process\n",
			tg.BotTokenEnv, tg.ChatIDEnv)
		return n
	}
	n.enabled = true
	n.instance = tg.InstanceName
	n.policy = tg.Policy
	n.sender = notify.NewSenderWithBase(token, chatID, telegramAPIBase())
	return n
}

// newCollector creates the round detail listing when notifications are on. A
// creation failure degrades the notification only.
func (n *notifier) newCollector(mode string) *notify.Collector {
	if n == nil || !n.enabled {
		return nil
	}
	collector, err := notify.NewCollector(runIdentifier(mode), n.threshold)
	if err != nil {
		fmt.Fprintf(n.stderr, "notifications: detail collection is unavailable; the notification will be degraded\n")
		return nil
	}
	return collector
}

// runIdentifier is the run label shared by the summary and the attachment.
func runIdentifier(mode string) string {
	return mode + " " + time.Now().Format(time.RFC3339)
}

// sendRound sends the one aggregate summary for a finished round, after every
// synchronization resource has been released. Busy skips and cancelled rounds
// never send. A send failure is recorded locally with the fixed redacted
// vocabulary and never changes the round result or exit code.
func (n *notifier) sendRound(ctx context.Context, mode string, started time.Time, syncDuration time.Duration, report *reconciler.Report, result scheduler.Result, collector *notify.Collector) {
	if n == nil || !n.enabled || collector == nil {
		return
	}
	// The temporary listing is cleaned up on every path, including busy skips
	// and cancelled rounds that never send.
	defer collector.Close()
	if result.Busy || ctx.Err() != nil {
		return
	}
	info := buildRunInfo(n, mode, started, syncDuration, report)
	anyChange := collector.HasChange()
	if collector == nil {
		anyChange = fallbackChange(report)
	}
	if !notify.ShouldSend(n.policy, info.Outcome, anyChange) {
		return
	}
	collector.Finalize()
	text, attachment := notify.BuildMessage(info, collector)
	if err := n.sender.Send(ctx, text, attachment); err != nil {
		if ctx.Err() != nil {
			fmt.Fprintf(n.stderr, "notifications: delivery stopped: %s\n", err)
			return
		}
		fmt.Fprintf(n.stderr, "notifications: telegram delivery failed: %s\n", err)
	}
}

// fallbackChange reports observable file-level changes from the report when no
// detail collector exists. Committed missing-count progress is unknown in that
// case and is reported as unavailable, never guessed.
func fallbackChange(report *reconciler.Report) bool {
	if report == nil {
		return false
	}
	for _, src := range report.Sources {
		if src.Created > 0 || src.Updated > 0 || src.Deleted > 0 || src.Reclaimed > 0 {
			return true
		}
	}
	return false
}

// buildRunInfo assembles the summary inputs from one finished round. A missing
// report means initialization failed after the notification assembly; counts
// are then reported as unavailable instead of fabricated.
func buildRunInfo(n *notifier, mode string, started time.Time, syncDuration time.Duration, report *reconciler.Report) notify.RunInfo {
	info := notify.RunInfo{
		InstanceName: n.instance,
		Mode:         mode,
		StartedAt:    started,
		FinishedAt:   started.Add(syncDuration),
		SyncDuration: syncDuration,
		Threshold:    n.threshold,
	}
	if report == nil {
		info.Outcome = notify.OutcomeInitFailure
		info.SyncDuration = syncDuration
		return info
	}
	info.StatsAvailable = true
	for _, src := range report.Sources {
		info.SourceCount++
		incomplete := src.Partial || src.Err != nil
		if incomplete {
			info.IncompleteSources++
		} else {
			info.CompletedSources++
		}
		info.Stats.Created += src.Created
		info.Stats.Updated += src.Updated
		info.Stats.Skipped += src.Skipped
		info.Stats.Filtered += src.Filtered
		info.Stats.Deleted += src.Deleted
		info.Stats.Reclaimed += src.Reclaimed
		info.Stats.Retained += src.Retained
		info.Stats.Conflicts += src.Conflicts
		info.Stats.Errors += src.Errors
		if incomplete {
			status := notify.SourceStatus{Index: src.SourceIndex}
			notes := append([]string(nil), src.Diagnostics...)
			if len(notes) > 5 {
				status.Notes = notes[:5]
				status.NotesMore = len(notes) - 5
			} else {
				status.Notes = notes
			}
			info.Sources = append(info.Sources, status)
		}
	}
	if info.IncompleteSources > 0 {
		info.Outcome = notify.OutcomeSourceFailure
	}
	return info
}
