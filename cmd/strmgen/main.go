// Command strmgen is the OpenList-to-STRM synchronizer entry point.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"sort"
	"syscall"
	"time"

	"strm-generator-go/internal/config"
	"strm-generator-go/internal/openlist"
	"strm-generator-go/internal/pathmap"
	"strm-generator-go/internal/reconciler"
	"strm-generator-go/internal/scheduler"
	"strm-generator-go/internal/storage"
)

// Exit codes are part of the CLI contract.
const (
	exitOK      = 0
	exitRuntime = 1
	exitUsage   = 2
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func usage(w io.Writer) {
	fmt.Fprint(w, `strmgen — OpenList to STRM synchronizer

Usage:
  strmgen sync --config <file> [--dry-run]
  strmgen serve --config <file>
  strmgen help

Commands:
  sync    Run one synchronization round for every configured source.
          --dry-run previews without modifying files, the database or rounds.
  serve   Run one immediate round, then repeat every sync.interval_minutes
          (default 60). SIGINT/SIGTERM stop after the current round without
          committing an incomplete round. A busy shared lock is reported as
          busy-skip and the next interval is still scheduled. Summary lines go
          to stdout; redacted diagnostics go to stderr.
  help    Show this message.

Exit codes:
  0  success
  1  runtime failure (API, file, database, conflict or busy), cancelled serve
  2  invalid arguments, invalid configuration or unsupported command
`)
}

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		usage(stderr)
		return exitUsage
	}
	switch args[0] {
	case "help", "-h", "--help":
		usage(stdout)
		return exitOK
	case "sync":
		return runSync(args[1:], stdout, stderr)
	case "serve":
		return runServe(args[1:], stdout, stderr)
	default:
		fmt.Fprintln(stderr, "unsupported command")
		usage(stderr)
		return exitUsage
	}
}

// roundDeps carries everything one sync round needs. The same assembly is used
// by the one-shot `sync` command and by every `serve` tick, so both share the
// fixed runtime lock, output-root preparation, store lifecycle and reconciler.
type roundDeps struct {
	resolved *config.Resolved
	dryRun   bool
	token    string
	stdout   io.Writer
	stderr   io.Writer
	// media is the extension set resolved once at startup. serve keeps the same
	// set for every tick, so editing the configuration file never changes a
	// running process; a restart is required.
	media *pathmap.MediaSet
}

// runRound executes exactly one synchronization round through the shared
// entry point. It acquires the fixed cross-process lock, prepares the output
// roots, opens the ownership store, runs the reconciler and releases every
// resource before returning. A busy lock is reported as scheduler.Result.Busy
// and never reaches the reconciler. Startup resource failures (lock resource,
// output root or database initialization) are fatal on the first round and a
// retryable failure on later serve ticks.
func runRound(ctx context.Context, deps roundDeps, first bool) (*reconciler.Report, scheduler.Result) {
	outputs := make([]string, 0, len(deps.resolved.Sources))
	for _, src := range deps.resolved.Sources {
		outputs = append(outputs, src.Output)
	}
	lock, err := reconciler.AcquireLockFor(outputs, deps.resolved.DatabasePath)
	if err != nil {
		if errors.Is(err, reconciler.ErrBusy) {
			return nil, scheduler.Result{Busy: true}
		}
		// A lock resource error is never recoverable in-process.
		return nil, scheduler.Result{Fatal: true, Err: errors.New("runtime lock is unavailable or unsafe")}
	}
	defer lock.Release()

	filesystem := &reconciler.LocalFS{}
	if err := filesystem.PrepareRoots(outputs); err != nil {
		return nil, startupOrFailure(first, errors.New("source output roots are unavailable or unsafe"))
	}
	defer filesystem.CloseRoots()

	store, err := openStore(deps.resolved, deps.dryRun)
	if err != nil {
		return nil, startupOrFailure(first, errors.New("database.path is unavailable or unsafe"))
	}
	if store != nil {
		defer store.Close()
	}

	client := openlist.New(deps.resolved.APIURL, deps.token)
	sources := make([]reconciler.Source, 0, len(deps.resolved.Sources))
	for _, src := range deps.resolved.Sources {
		sources = append(sources, reconciler.Source{
			Index:  src.Index,
			Key:    sourceIdentity(deps.resolved.APIURL, src.Remote, src.Output),
			Remote: src.Remote,
			Output: src.Output,
		})
	}
	var ownership reconciler.OwnershipStore
	if store != nil {
		ownership = store
	}
	syncer := reconciler.New(client, ownership, reconciler.Options{
		DryRun:               deps.dryRun,
		PublicPrefix:         deps.resolved.PublicPrefix,
		Workers:              deps.resolved.DirectoryWorkers,
		FS:                   filesystem,
		CleanupEnabled:       deps.resolved.CleanupEnabled,
		MissingConfirmations: deps.resolved.MissingConfirmations,
		MediaExtensions:      deps.media,
	})

	report, runErr := syncer.Run(ctx, sources)
	return report, scheduler.Result{Err: runErr}
}

// startupOrFailure marks an initialization error as fatal on the first round
// and retryable afterwards, so serve exits on a broken startup while a later
// transient resource failure is reported and retried.
func startupOrFailure(first bool, err error) scheduler.Result {
	if first {
		return scheduler.Result{Fatal: true, Err: err}
	}
	return scheduler.Result{Err: err}
}

func runSync(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("sync", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	configPath := fs.String("config", "", "path to the YAML configuration file")
	dryRun := fs.Bool("dry-run", false, "preview without modifying files, database or rounds")
	fs.Usage = func() { usage(stderr) }
	if err := fs.Parse(args); err != nil {
		fmt.Fprintln(stderr, "invalid sync arguments")
		return exitUsage
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(stderr, "sync accepts no positional arguments")
		return exitUsage
	}
	if *configPath == "" {
		fmt.Fprintln(stderr, "--config is required")
		return exitUsage
	}

	resolved, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintf(stderr, "configuration error: %s\n", config.SafeDiagnostic(err))
		return exitUsage
	}
	token, err := resolved.Token()
	if err != nil {
		fmt.Fprintln(stderr, "configuration error: configured token environment variable is not set")
		return exitUsage
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	report, result := runRound(ctx, roundDeps{resolved: resolved, dryRun: *dryRun, stdout: stdout, stderr: stderr, token: token, media: mediaSet(resolved)}, true)
	if report != nil {
		printReport(stdout, stderr, report, *dryRun)
	}
	switch {
	case result.Busy:
		fmt.Fprintln(stderr, "another sync is already running")
		return exitRuntime
	case result.Fatal:
		fmt.Fprintln(stderr, result.Err)
		return exitRuntime
	case ctx.Err() != nil:
		fmt.Fprintln(stderr, "sync cancelled")
		return exitRuntime
	case result.Err != nil:
		return exitRuntime
	}
	return exitOK
}

func runServe(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	configPath := fs.String("config", "", "path to the YAML configuration file")
	fs.Usage = func() { usage(stderr) }
	if err := fs.Parse(args); err != nil {
		fmt.Fprintln(stderr, "invalid serve arguments")
		return exitUsage
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(stderr, "serve accepts no positional arguments")
		return exitUsage
	}
	if *configPath == "" {
		fmt.Fprintln(stderr, "--config is required")
		return exitUsage
	}

	resolved, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintf(stderr, "configuration error: %s\n", config.SafeDiagnostic(err))
		return exitUsage
	}
	// serve-only interval validation runs before token lookup, lock
	// acquisition, output-root preparation, database open or any remote
	// request, so an interval that cannot drive the timer never causes a
	// product side effect. One-shot `sync` deliberately skips this check and
	// keeps accepting interval_minutes: 0.
	if err := resolved.ValidateServeInterval(); err != nil {
		fmt.Fprintf(stderr, "configuration error: %s\n", config.SafeDiagnostic(err))
		return exitUsage
	}
	token, err := resolved.Token()
	if err != nil {
		fmt.Fprintln(stderr, "configuration error: configured token environment variable is not set")
		return exitUsage
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	deps := roundDeps{resolved: resolved, dryRun: false, stdout: stdout, stderr: stderr, token: token, media: mediaSet(resolved)}
	var lastReport *reconciler.Report
	round := func(roundCtx context.Context, first bool) scheduler.Result {
		lastReport = nil
		report, result := runRound(roundCtx, deps, first)
		lastReport = report
		return result
	}
	interval := serveIntervalOverride(resolved.Interval)
	sched := scheduler.New(interval, round, nil, func(event scheduler.Event) {
		printServeSummary(stdout, stderr, event, lastReport)
		lastReport = nil
	})
	event := sched.Run(ctx)
	if event.Kind == scheduler.Cancelled {
		return exitRuntime
	}
	if event.Kind == scheduler.Fatal {
		return exitRuntime
	}
	return exitOK
}

// mediaSet builds the instance-owned extension set from the validated
// configuration. sync, sync --dry-run and serve all call it once, immediately
// after Load, so every entry point filters with the same resolved list.
func mediaSet(resolved *config.Resolved) *pathmap.MediaSet {
	return pathmap.NewMediaSet(resolved.MediaExtensions)
}

// openStore returns nil for a dry-run without a database so the syncer treats
// every path as unowned without creating any file.
func openStore(resolved *config.Resolved, dryRun bool) (*storage.Store, error) {
	if !dryRun {
		return storage.Open(resolved.DatabasePath)
	}
	outputs := make([]string, 0, len(resolved.Sources))
	for _, source := range resolved.Sources {
		outputs = append(outputs, source.Output)
	}
	store, err := storage.OpenReadOnly(resolved.DatabasePath, outputs...)
	if errors.Is(err, storage.ErrNoDatabase) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return store, nil
}

func printReport(stdout, stderr io.Writer, report *reconciler.Report, dryRun bool) {
	mode := "sync"
	if dryRun {
		mode = "dry-run"
	}
	fmt.Fprintf(stdout, "mode: %s\n", mode)
	for _, src := range report.Sources {
		fmt.Fprintf(stdout, "source %d: created=%d updated=%d skipped=%d conflicts=%d filtered=%d errors=%d partial=%t diagnostics_omitted=%d deleted=%d missing=%d reclaimed=%d retained=%d candidates=%d\n",
			src.SourceIndex, src.Created, src.Updated, src.Skipped, src.Conflicts, src.Filtered, src.Errors, src.Partial, src.DiagnosticsOmitted,
			src.Deleted, src.ConfirmedMissing, src.Reclaimed, src.Retained, src.Candidates)
		diagnostics := append([]string(nil), src.Diagnostics...)
		sort.Strings(diagnostics)
		for _, d := range diagnostics {
			fmt.Fprintf(stderr, "source %d: %s\n", src.SourceIndex, d)
		}
		if src.Err != nil {
			fmt.Fprintf(stderr, "source %d: source round incomplete\n", src.SourceIndex)
		}
	}
}

// serveSummary aggregates the per-source counters of one serve round.
type serveSummary struct {
	created   int
	updated   int
	skipped   int
	deleted   int
	reclaimed int
}

func summarize(report *reconciler.Report) serveSummary {
	var s serveSummary
	if report == nil {
		return s
	}
	for _, src := range report.Sources {
		s.created += src.Created
		s.updated += src.Updated
		s.skipped += src.Skipped
		s.deleted += src.Deleted
		s.reclaimed += src.Reclaimed
	}
	return s
}

// printServeSummary writes one bounded, redacted summary line to stdout and the
// per-source diagnostics to stderr. It never prints tokens, signed URLs or
// private full paths. The next scheduled time is fixed to RFC3339.
func printServeSummary(stdout, stderr io.Writer, event scheduler.Event, report *reconciler.Report) {
	s := summarize(report)
	cleanup := s.deleted + s.reclaimed
	next := "-"
	if !event.Next.IsZero() {
		next = event.Next.Format(time.RFC3339)
	}
	fmt.Fprintf(stdout, "mode: serve result: %s created=%d updated=%d skipped=%d cleanup=%d deleted=%d reclaimed=%d next=%s\n",
		event.Kind, s.created, s.updated, s.skipped, cleanup, s.deleted, s.reclaimed, next)
	if event.Err != nil && event.Kind != scheduler.Cancelled {
		fmt.Fprintf(stderr, "serve %s: %s\n", event.Kind, event.Err)
	}
	if report == nil {
		return
	}
	for _, src := range report.Sources {
		diagnostics := append([]string(nil), src.Diagnostics...)
		sort.Strings(diagnostics)
		for _, d := range diagnostics {
			fmt.Fprintf(stderr, "source %d: %s\n", src.SourceIndex, d)
		}
		if src.Err != nil {
			fmt.Fprintf(stderr, "source %d: source round incomplete\n", src.SourceIndex)
		}
	}
}

func sourceIdentity(apiURL, remote, output string) string {
	// Length framing avoids ambiguous string concatenation. Token and public URL
	// are intentionally excluded: credential rotation retains identity, while
	// public URL changes use the existing STRM URL comparison rule.
	payload := fmt.Sprintf("%d:%s%d:%s%d:%s", len(apiURL), apiURL, len(remote), remote, len(output), output)
	sum := sha256.Sum256([]byte(payload))
	return "source-v1:" + hex.EncodeToString(sum[:])
}
