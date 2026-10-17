// Command strmgen is the OpenList-to-STRM synchronizer entry point.
package main

import (
	"context"
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
		fmt.Fprintf(stderr, "unsupported command %q\n", args[0])
		usage(stderr)
		return exitUsage
	}
}

func runSync(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("sync", flag.ContinueOnError)
	fs.SetOutput(stderr)
	configPath := fs.String("config", "", "path to the YAML configuration file")
	dryRun := fs.Bool("dry-run", false, "preview without modifying files, database or rounds")
	fs.Usage = func() { usage(stderr) }
	if err := fs.Parse(args); err != nil {
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
		fmt.Fprintln(stderr, "configuration error:", err)
		return exitUsage
	}
	token, err := resolved.Token()
	if err != nil {
		fmt.Fprintln(stderr, "configuration error:", err)
		return exitUsage
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	lock, err := reconciler.AcquireLock()
	if err != nil {
		if errors.Is(err, reconciler.ErrBusy) {
			fmt.Fprintln(stderr, "another sync is already running")
			return exitRuntime
		}
		fmt.Fprintln(stderr, "cannot acquire sync lock:", err)
		return exitRuntime
	}
	defer lock.Release()

	store, err := openStore(resolved, *dryRun)
	if err != nil {
		fmt.Fprintln(stderr, "database error:", err)
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
			Key:    src.Remote,
			Remote: src.Remote,
			Output: src.Output,
		})
	}
	var ownership reconciler.OwnershipStore
	if store != nil {
		ownership = store
	}
	syncer := reconciler.New(client, ownership, reconciler.Options{
		DryRun:       *dryRun,
		PublicPrefix: resolved.PublicPrefix,
		Workers:      resolved.DirectoryWorkers,
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
	store, err := storage.OpenReadOnly(resolved.DatabasePath)
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
		fmt.Fprintf(stdout, "source %d %s: created=%d skipped=%d conflicts=%d needs_update=%d filtered=%d errors=%d\n",
			src.SourceIndex, src.Remote, src.Created, src.Skipped, src.Conflicts, src.NeedsUpdate, src.Filtered, src.Errors)
		diagnostics := append([]string(nil), src.Diagnostics...)
		sort.Strings(diagnostics)
		for _, d := range diagnostics {
			fmt.Fprintf(stderr, "source %d %s: %s\n", src.SourceIndex, src.Remote, d)
		}
		if src.Err != nil {
			fmt.Fprintf(stderr, "source %d %s: error: %v\n", src.SourceIndex, src.Remote, src.Err)
		}
	}
}
