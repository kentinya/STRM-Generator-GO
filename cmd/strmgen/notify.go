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

// factSinkAdapter forwards reconciler facts to the notify collector without
// coupling the reconciler or the collector to each other. Missing-count
// advances arrive while the commit transaction runs; the collector holds them
// as provisional until the commit is decided, so an uncommitted candidate can
// never be published as progress.
type factSinkAdapter struct {
	collector *notify.Collector
}

func (a factSinkAdapter) Record(fact reconciler.Fact) {
	event := notify.Event{
		SourceIndex:  fact.SourceIndex,
		Kind:         notifyEventKind(fact.Kind),
		RelativePath: fact.RelativePath,
		PriorCount:   fact.PriorCount,
		NewCount:     fact.NewCount,
		Reason:       fact.Reason,
	}
	if fact.Kind == reconciler.FactMissingAdvanced {
		// Missing-count advances arrive inside the commit transaction window;
		// they must stay provisional so a failed or rolled-back commit never
		// publishes an uncommitted candidate as progress.
		a.collector.RecordProvisional(event)
		return
	}
	a.collector.Record(event)
}

// CommitStaged confirms the provisional advance records of one commit attempt.
func (a factSinkAdapter) CommitStaged() { a.collector.CommitProvisional() }

// DiscardStaged removes the provisional advance records after a failed commit.
func (a factSinkAdapter) DiscardStaged() { a.collector.DiscardProvisional() }

// notifyEventKind maps the reconciler fact vocabulary onto the notify event
// vocabulary. The two packages stay independent by design.
func notifyEventKind(kind reconciler.FactKind) notify.Kind {
	switch kind {
	case reconciler.FactCreated:
		return notify.KindCreated
	case reconciler.FactUpdated:
		return notify.KindUpdated
	case reconciler.FactDeleted:
		return notify.KindDeleted
	case reconciler.FactReclaimed:
		return notify.KindReclaimed
	case reconciler.FactMissingCleared:
		return notify.KindMissingCleared
	case reconciler.FactMissingAdvanced:
		return notify.KindMissingAdvanced
	default:
		return ""
	}
}

// notifier owns the process-local notification state. A disabled notifier
// never reads the notification environment variables, never creates a
// temporary listing and never sends anything.
type notifier struct {
	enabled   bool
	sender    *notify.Sender
	instance  string
	policy    string
	threshold int
	// sources is the configured source count; it stays known even when a
	// round never produced a report.
	sources int
	stderr  io.Writer
}

// setupNotifications applies the startup credential contract: a disabled
// notification configuration is inert, and an enabled configuration that is
// missing credentials only records a redacted warning and disables
// notifications for this process. dry-run never reads the credential
// environment variables.
func setupNotifications(resolved *config.Resolved, dryRun bool, stderr io.Writer) *notifier {
	n := &notifier{stderr: stderr, threshold: resolved.MissingConfirmations, sources: len(resolved.Sources)}
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
// creation failure degrades the notification only: the summary is still sent
// with a clear incompleteness note, and the missing-count progress is
// reported as unknown instead of zero.
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
	if n == nil || !n.enabled {
		return
	}
	// The temporary listing is cleaned up on every path, including busy skips
	// and cancelled rounds that never send.
	if collector != nil {
		defer collector.Close()
	}
	if result.Busy || ctx.Err() != nil {
		return
	}
	info := buildRunInfo(n, mode, started, syncDuration, report)
	info.DetailUnavailable = collector == nil
	anyChange := collector != nil && collector.HasChange()
	if collector == nil {
		// Without detail collection only the report counters are observable;
		// committed missing-count progress is unknown and is never guessed.
		anyChange = fallbackChange(report)
	}
	if !notify.ShouldSend(n.policy, info.Outcome, anyChange) {
		return
	}
	// The message is built before the trailer is finalized: the body budget
	// may append omitted source details to the same single attachment.
	// BuildMessage appends the overflow source details and writes the final
	// trailer itself, so the tail notes and the attachment decision reflect
	// the real final listing state, including a failed trailer write.
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
// are then reported as unavailable instead of fabricated, while the configured
// source count stays known. Completed sources with retained rows or conflicts
// are also listed so their safety reasons are never silently dropped.
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
		// The configured source count stays known even without a report; the
		// per-source completion and all counters are unavailable.
		info.SourceCount = n.sources
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
			info.Sources = append(info.Sources, sourceStatus(src, false))
		} else if src.Retained > 0 || src.Conflicts > 0 {
			// A completed source can still preserve rows for a safety reason;
			// its established diagnostic vocabulary explains why.
			info.Sources = append(info.Sources, sourceStatus(src, true))
		}
	}
	if info.IncompleteSources > 0 {
		info.Outcome = notify.OutcomeSourceFailure
	}
	return info
}

// sourceStatus bounds one source's diagnostics for the body (five notes plus
// an omission count) and keeps every sampled diagnostic for the attachment.
func sourceStatus(src reconciler.SourceResult, completed bool) notify.SourceStatus {
	status := notify.SourceStatus{Index: src.SourceIndex, Completed: completed, Retained: src.Retained}
	notes := append([]string(nil), src.Diagnostics...)
	status.DetailNotes = notes
	if len(notes) > 5 {
		status.Notes = notes[:5]
		status.NotesMore = len(notes) - 5 + src.DiagnosticsOmitted
	} else {
		status.Notes = notes
		status.NotesMore = src.DiagnosticsOmitted
	}
	return status
}
