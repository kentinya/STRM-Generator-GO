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

	"strm-generator-go/internal/config"
	"strm-generator-go/internal/openlist"
	"strm-generator-go/internal/reconciler"
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
  strmgen help

Commands:
  sync    Run one synchronization round for every configured source.
          --dry-run previews without modifying files, the database or rounds.
  help    Show this message.

Exit codes:
  0  success
  1  runtime failure (API, file, database, conflict or busy)
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
		fmt.Fprintln(stderr, "serve is not supported by this task")
		return exitUsage
	default:
		fmt.Fprintln(stderr, "unsupported command")
		usage(stderr)
		return exitUsage
	}
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

	outputs := make([]string, 0, len(resolved.Sources))
	for _, src := range resolved.Sources {
		outputs = append(outputs, src.Output)
	}
	lock, err := reconciler.AcquireLockFor(outputs, resolved.DatabasePath)
	if err != nil {
		if errors.Is(err, reconciler.ErrBusy) {
			fmt.Fprintln(stderr, "another sync is already running")
			return exitRuntime
		}
		fmt.Fprintln(stderr, "runtime lock is unavailable or unsafe")
		return exitRuntime
	}
	defer lock.Release()
	filesystem := &reconciler.LocalFS{}
	if err := filesystem.PrepareRoots(outputs); err != nil {
		fmt.Fprintln(stderr, "source output roots are unavailable or unsafe")
		return exitRuntime
	}
	defer filesystem.CloseRoots()

	store, err := openStore(resolved, *dryRun)
	if err != nil {
		fmt.Fprintln(stderr, "database.path is unavailable or unsafe")
		return exitRuntime
	}
	if store != nil {
		defer store.Close()
	}

	client := openlist.New(resolved.APIURL, token)
	sources := make([]reconciler.Source, 0, len(resolved.Sources))
	for _, src := range resolved.Sources {
		sources = append(sources, reconciler.Source{
			Index:  src.Index,
			Key:    sourceIdentity(resolved.APIURL, src.Remote, src.Output),
			Remote: src.Remote,
			Output: src.Output,
		})
	}
	var ownership reconciler.OwnershipStore
	if store != nil {
		ownership = store
	}
	syncer := reconciler.New(client, ownership, reconciler.Options{
		DryRun:               *dryRun,
		PublicPrefix:         resolved.PublicPrefix,
		Workers:              resolved.DirectoryWorkers,
		FS:                   filesystem,
		CleanupEnabled:       resolved.CleanupEnabled,
		MissingConfirmations: resolved.MissingConfirmations,
	})

	report, runErr := syncer.Run(ctx, sources)
	printReport(stdout, stderr, report, *dryRun)
	if ctx.Err() != nil {
		fmt.Fprintln(stderr, "sync cancelled")
		return exitRuntime
	}
	if runErr != nil {
		return exitRuntime
	}
	return exitOK
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

func sourceIdentity(apiURL, remote, output string) string {
	// Length framing avoids ambiguous string concatenation. Token and public URL
	// are intentionally excluded: credential rotation retains identity, while
	// public URL changes use the existing STRM URL comparison rule.
	payload := fmt.Sprintf("%d:%s%d:%s%d:%s", len(apiURL), apiURL, len(remote), remote, len(output), output)
	sum := sha256.Sum256([]byte(payload))
	return "source-v1:" + hex.EncodeToString(sum[:])
}
