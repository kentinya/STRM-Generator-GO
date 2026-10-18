package reconciler

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"strm-generator-go/internal/config"
	"strm-generator-go/internal/openlist"
	"strm-generator-go/internal/pathmap"
	"strm-generator-go/internal/storage"
	"strm-generator-go/internal/strm"
)

// Lister is the subset of the OpenList client the syncer uses.
type Lister interface {
	Get(ctx context.Context, remotePath string) (*openlist.Entry, error)
	List(ctx context.Context, remotePath string) ([]openlist.Entry, error)
}

// OwnershipStore is the durable ownership/round backend.
type OwnershipStore interface {
	Lookup(sourceKey, relativePath string) (storage.Ownership, bool, error)
	RecordOwnership(ctx context.Context, o storage.Ownership, epoch int64) error
	NextEpoch(sourceKey string) (int64, error)
	CommitRound(ctx context.Context, sourceKey string, epoch int64, at time.Time) error
}

// Source is one resolved source to synchronize.
type Source struct {
	Index  int
	Key    string
	Remote string
	Output string
}

// Options configures a sync round.
type Options struct {
	DryRun       bool
	PublicPrefix string
	Workers      int
	FS           FileSystem
	Now          func() time.Time
}

// SourceResult summarizes one source round.
type SourceResult struct {
	SourceIndex        int
	Remote             string
	Created            int
	Updated            int
	Skipped            int
	Conflicts          int
	Filtered           int
	Errors             int
	Partial            bool
	Diagnostics        []string
	DiagnosticsOmitted int
	Err                error
}

const maxDiagnosticSamples = 100

func (r *SourceResult) addDiagnostic(message string) {
	if len(r.Diagnostics) < maxDiagnosticSamples {
		r.Diagnostics = append(r.Diagnostics, message)
		return
	}
	r.DiagnosticsOmitted++
}

// Report summarizes a complete run.
type Report struct {
	Sources   []SourceResult
	DryRun    bool
	HadErrors bool
}

// Syncer executes a bounded single round per source.
type Syncer struct {
	lister Lister
	store  OwnershipStore
	opts   Options
}

// New builds a Syncer.
func New(lister Lister, store OwnershipStore, opts Options) *Syncer {
	if opts.FS == nil {
		opts.FS = &LocalFS{}
	}
	if opts.Now == nil {
		opts.Now = time.Now
	}
	if opts.Workers < 1 {
		opts.Workers = config.DefaultDirectoryWorkers
	}
	return &Syncer{lister: lister, store: store, opts: opts}
}

// Run synchronizes every source. It returns a report even when a source
// fails; the error is non-nil when any source did not complete successfully.
func (s *Syncer) Run(ctx context.Context, sources []Source) (*Report, error) {
	report := &Report{DryRun: s.opts.DryRun}
	if preparer, ok := s.opts.FS.(interface {
		PrepareRoots([]string) error
		CloseRoots() error
	}); ok {
		roots := make([]string, 0, len(sources))
		for _, src := range sources {
			roots = append(roots, src.Output)
		}
		if err := preparer.PrepareRoots(roots); err != nil {
			for _, src := range sources {
				res := SourceResult{
					SourceIndex: src.Index,
					Remote:      src.Remote,
					Errors:      1,
					Partial:     true,
					Err:         errors.New("output root is unavailable or unsafe"),
				}
				res.addDiagnostic("cannot securely bind output root; no round committed")
				report.Sources = append(report.Sources, res)
			}
			report.HadErrors = true
			return report, errors.New("one or more output roots are unavailable or unsafe")
		}
		defer preparer.CloseRoots()
	}
	for _, src := range sources {
		res := s.runSource(ctx, src)
		report.Sources = append(report.Sources, res)
		if res.Err != nil || res.Partial {
			report.HadErrors = true
		}
	}
	if report.HadErrors {
		return report, errors.New("one or more sources failed")
	}
	return report, nil
}

func (s *Syncer) runSource(ctx context.Context, src Source) SourceResult {
	res := SourceResult{SourceIndex: src.Index, Remote: src.Remote}
	// Verify the remote root is readable before walking it. A failed get must
	// not be interpreted as an empty directory.
	root, err := s.lister.Get(ctx, src.Remote)
	if err != nil {
		res.Err = fmt.Errorf("read source root: %w", err)
		res.Partial = true
		res.Errors++
		res.addDiagnostic("source root not readable; no round committed")
		return res
	}
	if root == nil || !root.IsDir {
		res.Err = errors.New("source root is not a directory")
		res.Partial = true
		res.Errors++
		res.addDiagnostic("source root response is not a directory; no round committed")
		return res
	}

	traversal := &directoryWalker{
		workers: s.opts.Workers,
		lister:  s.lister,
		root:    src.Remote,
		onDir: func(remoteDir, relDir string, entries []openlist.Entry) {
			s.handleDirectory(ctx, src, remoteDir, relDir, entries, &res)
		},
	}
	if err := traversal.run(ctx); err != nil {
		res.Err = err
		res.Partial = true
		res.Errors++
		res.addDiagnostic("incomplete traversal; no round committed")
		return res
	}
	if res.Conflicts > 0 || res.Errors > 0 {
		res.Partial = true
		res.addDiagnostic("round not committed because of conflicts or errors")
		return res
	}
	if s.opts.DryRun {
		return res
	}
	if err := ctx.Err(); err != nil {
		res.Err = err
		res.Partial = true
		res.Errors++
		res.addDiagnostic("sync cancelled; no round committed")
		return res
	}
	if s.store != nil {
		epoch, err := s.store.NextEpoch(src.Key)
		if err != nil {
			res.Err = err
			res.Partial = true
			return res
		}
		if err := s.store.CommitRound(ctx, src.Key, epoch, s.opts.Now()); err != nil {
			res.Err = err
			res.Partial = true
			res.Errors++
			res.addDiagnostic("round commit failed; source round not committed")
			return res
		}
	}
	return res
}

// handleDirectory filters, detects same-target conflicts within one directory,
// then processes each surviving file. Conflict detection is bounded to a
// single directory listing and never selects one input arbitrarily.
func (s *Syncer) handleDirectory(ctx context.Context, src Source, dir, relDir string, entries []openlist.Entry, res *SourceResult) {
	type candidate struct {
		entry openlist.Entry
		rel   string
	}
	byRel := map[string][]string{}
	var candidates []candidate
	for _, entry := range entries {
		if entry.IsDir || !pathmap.IsMedia(entry.Name) {
			if !entry.IsDir {
				res.Filtered++
			}
			continue
		}
		rel, err := pathmap.LocalRelative(relDir, entry.Name)
		if err != nil {
			res.Errors++
			res.addDiagnostic("invalid file name skipped")
			continue
		}
		byRel[rel] = append(byRel[rel], entry.Name)
		candidates = append(candidates, candidate{entry: entry, rel: rel})
	}

	conflicted := map[string]bool{}
	for rel, names := range byRel {
		if len(names) > 1 {
			conflicted[rel] = true
			res.Conflicts++
			res.addDiagnostic(fmt.Sprintf("target conflict %s from %s", rel, strings.Join(sortedStrings(names), ", ")))
		}
	}

	for _, c := range candidates {
		if conflicted[c.rel] {
			continue
		}
		s.handleFile(ctx, src, dir, relDir, c.entry, res)
	}
}

func (s *Syncer) handleFile(ctx context.Context, src Source, dir, relDir string, entry openlist.Entry, res *SourceResult) {
	rel, err := pathmap.LocalRelative(relDir, entry.Name)
	if err != nil {
		res.Errors++
		res.addDiagnostic("invalid file name skipped")
		return
	}
	remotePath := config.JoinRemote(dir, entry.Name)
	url, err := pathmap.StableURL(s.opts.PublicPrefix, remotePath)
	if err != nil {
		res.Errors++
		res.addDiagnostic(fmt.Sprintf("cannot build URL for %s", rel))
		return
	}
	desired := &strm.Document{URL: url}
	if entry.HasSHA {
		desired.SHA1, desired.HasSHA = entry.SHA1, true
	}
	if entry.Size != nil {
		desired.Size, desired.HasSize = *entry.Size, true
	}
	if err := desired.Validate(); err != nil {
		res.Errors++
		res.addDiagnostic(fmt.Sprintf("invalid metadata for %s", rel))
		return
	}
	if entry.Anomaly != "" {
		res.addDiagnostic(fmt.Sprintf("%s: %s", rel, entry.Anomaly))
	}

	targetAbs := filepath.Join(src.Output, filepath.FromSlash(rel))
	owned := false
	if s.store != nil {
		o, ok, err := s.store.Lookup(src.Key, rel)
		if err != nil {
			res.Errors++
			res.addDiagnostic(fmt.Sprintf("ownership lookup failed for %s", rel))
			return
		}
		owned = ok && o.AbsolutePath == targetAbs
	}

	info, statErr := s.opts.FS.Lstat(src.Output, rel)
	exists := statErr == nil
	if statErr != nil && !errors.Is(statErr, os.ErrNotExist) {
		res.Errors++
		res.addDiagnostic(fmt.Sprintf("cannot stat %s", rel))
		return
	}
	if exists && info.Mode()&os.ModeSymlink != 0 {
		res.Conflicts++
		res.addDiagnostic(fmt.Sprintf("refusing symlink target %s", rel))
		return
	}

	if exists && !owned {
		res.Conflicts++
		res.addDiagnostic(fmt.Sprintf("existing file not owned by this source: %s", rel))
		return
	}

	if exists && owned {
		raw, err := s.opts.FS.ReadFile(src.Output, rel)
		if err != nil {
			res.Conflicts++
			res.addDiagnostic(fmt.Sprintf("cannot read owned file %s", rel))
			return
		}
		existing, perr := strm.Parse(raw)
		if perr != nil {
			res.Conflicts++
			res.addDiagnostic(fmt.Sprintf("owned file is not a valid STRM: %s", rel))
			return
		}
		result, cerr := strm.Compare(existing, desired)
		if cerr != nil {
			res.Conflicts++
			res.addDiagnostic(fmt.Sprintf("owned file is not comparable: %s", rel))
			return
		}
		switch result.Decision {
		case strm.Unchanged:
			res.Skipped++
			if result.Anomaly != "" {
				res.addDiagnostic(fmt.Sprintf("%s: %s", rel, result.Anomaly))
			}
			return
		default:
			// A trusted owned target requires new content. In dry-run this is a
			// preview; otherwise the target is atomically replaced while
			// preserving the previous ownership mapping.
			if s.opts.DryRun {
				res.Updated++
				res.addDiagnostic(fmt.Sprintf("would update %s", rel))
				return
			}
			if err := s.opts.FS.Update(src.Output, rel, raw, desired.Bytes()); err != nil {
				res.Errors++
				res.addDiagnostic(fmt.Sprintf("update failed for %s", rel))
				return
			}
			res.Updated++
			return
		}
	}

	// Target does not exist. Plan or perform creation.
	if s.opts.DryRun {
		res.Created++
		res.addDiagnostic(fmt.Sprintf("would create %s", rel))
		return
	}
	if err := s.opts.FS.EnsureDir(src.Output, parentDir(rel)); err != nil {
		res.Errors++
		res.addDiagnostic(fmt.Sprintf("cannot create directory for %s", rel))
		return
	}
	if err := s.opts.FS.Publish(src.Output, rel, desired.Bytes()); err != nil {
		res.Errors++
		res.addDiagnostic(fmt.Sprintf("publish failed for %s", rel))
		return
	}
	res.Created++
	if s.store != nil {
		epoch, err := s.store.NextEpoch(src.Key)
		if err != nil {
			res.Errors++
			res.addDiagnostic(fmt.Sprintf("round lookup failed after publishing %s", rel))
			return
		}
		if err := s.store.RecordOwnership(ctx, storage.Ownership{
			SourceKey:    src.Key,
			RelativePath: rel,
			AbsolutePath: targetAbs,
		}, epoch); err != nil {
			// The file stays on disk but is not owned; a later run must treat
			// it as an unowned conflict.
			res.Errors++
			res.addDiagnostic(fmt.Sprintf("published but ownership not recorded: %s", rel))
			return
		}
		// afterOwnershipRecord is inert in release builds. The optional
		// strmgen_testhooks build can stop deterministically at exactly this
		// window (ownership durable, round not yet committed) for recovery
		// verification without a timing race.
		afterOwnershipRecord(rel)
	}
}

func parentDir(rel string) string {
	idx := strings.LastIndex(rel, "/")
	if idx < 0 {
		return ""
	}
	return rel[:idx]
}

func sortedStrings(in []string) []string {
	out := append([]string(nil), in...)
	sort.Strings(out)
	return out
}

type listResult struct {
	dir     string
	relDir  string
	entries []openlist.Entry
	err     error
}

// directoryWalker walks a remote tree breadth-first with a bounded worker
// pool. Only directory path strings are queued, never full media objects. The
// coordinator always offers a receive case alongside a send case, so a full
// jobs channel can never deadlock, and the pending directory queue is capped
// so a pathologically wide tree fails explicitly instead of growing memory
// without bound.
type directoryWalker struct {
	workers int
	lister  Lister
	root    string
	onDir   func(remoteDir, relDir string, entries []openlist.Entry)
}

// maxPendingDirs caps queued-but-not-yet-listed directories.
const maxPendingDirs = 10000

type walkJob struct {
	remote string
	rel    string
}

func (w *directoryWalker) run(ctx context.Context) error {
	jobs := make(chan walkJob, w.workers)
	results := make(chan listResult, w.workers)
	var wg sync.WaitGroup
	for i := 0; i < w.workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for job := range jobs {
				entries, err := w.lister.List(ctx, job.remote)
				results <- listResult{dir: job.remote, relDir: job.rel, entries: entries, err: err}
			}
		}()
	}

	queue := []walkJob{{remote: w.root, rel: ""}}
	inFlight := 0
	var firstErr error
	stopped := false

	for len(queue) > 0 || inFlight > 0 {
		var sendCh chan walkJob
		var sendVal walkJob
		if !stopped && len(queue) > 0 {
			sendCh = jobs
			sendVal = queue[0]
		}
		var recvCh chan listResult
		if inFlight > 0 {
			recvCh = results
		}
		if sendCh == nil && recvCh == nil {
			break
		}
		select {
		case sendCh <- sendVal:
			queue[0] = walkJob{}
			queue = queue[1:]
			inFlight++
		case r := <-recvCh:
			inFlight--
			if r.err != nil {
				if firstErr == nil {
					firstErr = r.err
				}
				stopped = true
				continue
			}
			if stopped {
				continue
			}
			w.onDir(r.dir, r.relDir, r.entries)
			for _, entry := range r.entries {
				if !entry.IsDir {
					continue
				}
				if err := pathmap.ValidateSegment(entry.Name); err != nil {
					if firstErr == nil {
						firstErr = fmt.Errorf("invalid directory name")
					}
					stopped = true
					break
				}
				if len(queue) >= maxPendingDirs {
					if firstErr == nil {
						firstErr = fmt.Errorf("directory queue limit exceeded")
					}
					stopped = true
					break
				}
				childRel := entry.Name
				if r.relDir != "" {
					childRel = r.relDir + "/" + entry.Name
				}
				if len(childRel) > pathmap.MaxRelativePathBytes {
					if firstErr == nil {
						firstErr = errors.New("relative directory path exceeds limit")
					}
					stopped = true
					break
				}
				remote := config.JoinRemote(r.dir, entry.Name)
				if len(remote) > pathmap.MaxRelativePathBytes {
					if firstErr == nil {
						firstErr = errors.New("remote directory path exceeds limit")
					}
					stopped = true
					break
				}
				queue = append(queue, walkJob{remote: remote, rel: childRel})
			}
		}
	}
	close(jobs)
	wg.Wait()
	return firstErr
}
