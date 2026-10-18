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

// Confirmer reads the current remote facts for one exact owned target under a
// configured source root. The syncer reuses openlist.Confirmer so the counting
// and the deletion rules can never drift apart.
type Confirmer interface {
	Confirm(ctx context.Context, sourceRoot, targetPath string) openlist.AbsenceResult
}

// OwnershipStore is the durable ownership/round backend plus the bounded
// round workspace used for observations and pending missing paths.
type OwnershipStore interface {
	Lookup(sourceKey, relativePath string) (storage.Ownership, bool, error)
	RecordOwnership(ctx context.Context, o storage.Ownership, epoch int64) error
	ClearMissing(ctx context.Context, sourceKey, relativePath string) error
	RemoveOwnership(ctx context.Context, sourceKey, relativePath string) error
	ListOwnershipAfter(ctx context.Context, sourceKey, after string, limit int) ([]storage.Ownership, error)
	ListEligibleAfter(ctx context.Context, sourceKey, after string, threshold, limit int) ([]storage.Ownership, error)
	CountUnmappedMissing(ctx context.Context, sourceKey string) (int64, error)
	PreciseMappingAvailable() bool
	ResetRound(ctx context.Context, sourceKey string) error
	MarkObserved(ctx context.Context, sourceKey, relativePath string) error
	IsObserved(ctx context.Context, sourceKey, relativePath string) (bool, error)
	AddPendingMissing(ctx context.Context, sourceKey string, relativePaths []string) error
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
	// CleanupEnabled admits missing observations and deletion. It is false by
	// default; configuration rejects mirror=false together with cleanup.
	CleanupEnabled bool
	// MissingConfirmations is the minimum committed consecutive complete-round
	// missing count before a target is eligible. The configuration enforces a
	// minimum of 3.
	MissingConfirmations int
	// Confirmer overrides the default openlist.Confirmer built over the lister.
	Confirmer Confirmer
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
	// ConfirmedMissing counts the missing observations staged for this round.
	// They only reach the durable counts when the round commits.
	ConfirmedMissing int
	// Deleted counts unlinks actually performed by this round.
	Deleted int
	// Reclaimed counts owned rows whose target was already gone locally and
	// whose directory entry was flushed before ownership was dropped.
	Reclaimed int
	// Retained counts rows preserved for a local safety reason (mapping,
	// type, staging, type conflict or an unmappable legacy record).
	Retained int
	// Candidates counts dry-run cleanup candidates that satisfied the current
	// committed threshold, the current remote rule and the local checks.
	Candidates int
}

const maxDiagnosticSamples = 100

// pendingBatchSize bounds every ownership enumeration and staged insert batch.
const pendingBatchSize = 200

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
	lister    Lister
	store     OwnershipStore
	opts      Options
	confirmer Confirmer
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
	confirmer := opts.Confirmer
	if confirmer == nil && lister != nil {
		confirmer = openlist.NewConfirmer(lister)
	}
	return &Syncer{lister: lister, store: store, opts: opts, confirmer: confirmer}
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
	// not be interpreted as an empty directory. The same gate the absence
	// confirmer uses decides this, so the reconciler can never accept a root the
	// cleanup rule would reject (or vice versa).
	switch reason := openlist.CheckSourceRoot(ctx, s.lister, src.Remote); reason {
	case openlist.ReasonNone:
	case openlist.ReasonCancelled:
		res.Err = ctx.Err()
		if res.Err == nil {
			res.Err = errors.New("source round cancelled")
		}
		res.Partial = true
		res.Errors++
		res.addDiagnostic("sync cancelled before reading the source root; no round committed")
		return res
	default:
		res.Err = fmt.Errorf("read source root: %s", reason)
		res.Partial = true
		res.Errors++
		res.addDiagnostic("source root not readable as a directory; no round committed")
		return res
	}

	var epoch int64
	if s.store != nil && !s.opts.DryRun {
		var err error
		epoch, err = s.store.NextEpoch(src.Key)
		if err != nil {
			res.Err = err
			res.Partial = true
			res.Errors++
			res.addDiagnostic("round lookup failed; source round not committed")
			return res
		}
		// Discard any observation or staged missing path from an earlier
		// interrupted attempt so it can never feed this round.
		if err := s.store.ResetRound(ctx, src.Key); err != nil {
			res.Err = err
			res.Partial = true
			res.Errors++
			res.addDiagnostic("round workspace reset failed; source round not committed")
			return res
		}
	}

	traversal := &directoryWalker{
		workers: s.opts.Workers,
		lister:  s.lister,
		root:    src.Remote,
		onDir: func(remoteDir, relDir string, entries []openlist.Entry) error {
			return s.handleDirectory(ctx, src, remoteDir, relDir, entries, epoch, &res)
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
	if ctx.Err() != nil {
		res.Err = ctx.Err()
		res.Partial = true
		res.Errors++
		res.addDiagnostic("sync cancelled; no round committed")
		return res
	}
	if s.opts.DryRun {
		if s.opts.CleanupEnabled {
			if err := s.previewCleanup(ctx, src, &res); err != nil {
				res.Err = err
				res.Partial = true
				res.Errors++
				res.addDiagnostic("cleanup preview stopped; no round committed")
			}
		}
		return res
	}
	if s.opts.CleanupEnabled && s.store != nil {
		if err := s.confirmAndCleanup(ctx, src, &res); err != nil {
			res.Err = err
			res.Partial = true
			res.Errors++
			res.addDiagnostic("cleanup stopped; source round not committed")
			return res
		}
	}
	if s.store != nil {
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

// noteObserved records one observed target. Marking the private observation is
// always safe; clearing the durable count is a conservative write that happens
// even when a later step of the round fails.
func (s *Syncer) noteObserved(ctx context.Context, src Source, rel string) error {
	if s.store == nil {
		return nil
	}
	if err := s.store.MarkObserved(ctx, src.Key, rel); err != nil {
		return err
	}
	if s.opts.DryRun {
		return nil
	}
	return s.store.ClearMissing(ctx, src.Key, rel)
}

// handleDirectory filters, detects same-target conflicts within one directory,
// records every observed target, then processes each surviving file.
// Conflict detection is bounded to a single directory listing and never
// selects one input arbitrarily. A dependency error aborts the round.
func (s *Syncer) handleDirectory(ctx context.Context, src Source, dir, relDir string, entries []openlist.Entry, epoch int64, res *SourceResult) error {
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
		if err := s.noteObserved(ctx, src, rel); err != nil {
			return err
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
		if err := s.handleFile(ctx, src, dir, relDir, c.entry, epoch, res); err != nil {
			return err
		}
	}
	return nil
}

func (s *Syncer) handleFile(ctx context.Context, src Source, dir, relDir string, entry openlist.Entry, epoch int64, res *SourceResult) error {
	rel, err := pathmap.LocalRelative(relDir, entry.Name)
	if err != nil {
		res.Errors++
		res.addDiagnostic("invalid file name skipped")
		return nil
	}
	remotePath := config.JoinRemote(dir, entry.Name)
	url, err := pathmap.StableURL(s.opts.PublicPrefix, remotePath)
	if err != nil {
		res.Errors++
		res.addDiagnostic(fmt.Sprintf("cannot build URL for %s", rel))
		return nil
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
		return nil
	}
	if entry.Anomaly != "" {
		res.addDiagnostic(fmt.Sprintf("%s: %s", rel, entry.Anomaly))
	}

	targetAbs := filepath.Join(src.Output, filepath.FromSlash(rel))
	owned := false
	if s.store != nil {
		o, ok, err := s.store.Lookup(src.Key, rel)
		if err != nil {
			return err
		}
		owned = ok && o.AbsolutePath == targetAbs
	}

	info, statErr := s.opts.FS.Lstat(src.Output, rel)
	exists := statErr == nil
	if statErr != nil && !errors.Is(statErr, os.ErrNotExist) {
		res.Errors++
		res.addDiagnostic(fmt.Sprintf("cannot stat %s", rel))
		return nil
	}
	if exists && info.Mode()&os.ModeSymlink != 0 {
		res.Conflicts++
		res.addDiagnostic(fmt.Sprintf("refusing symlink target %s", rel))
		return nil
	}

	// Reconcile any deterministic staging file left by an interrupted atomic
	// update before comparing content. Dry-run only detects it so it stays
	// side-effect free. The target's existence and type do not change here.
	if recErr := s.opts.FS.Recover(src.Output, rel, desired.Bytes(), owned, s.opts.DryRun); recErr != nil {
		res.Conflicts++
		res.addDiagnostic(fmt.Sprintf("unresolved interrupted update for %s", rel))
		return nil
	}

	if exists && !owned {
		res.Conflicts++
		res.addDiagnostic(fmt.Sprintf("existing file not owned by this source: %s", rel))
		return nil
	}

	refreshOwnership := func() error {
		if s.store == nil || s.opts.DryRun {
			return nil
		}
		return s.store.RecordOwnership(ctx, storage.Ownership{
			SourceKey:    src.Key,
			RelativePath: rel,
			AbsolutePath: targetAbs,
			RemotePath:   remotePath,
		}, epoch)
	}

	if exists && owned {
		raw, err := s.opts.FS.ReadFile(src.Output, rel)
		if err != nil {
			res.Conflicts++
			res.addDiagnostic(fmt.Sprintf("cannot read owned file %s", rel))
			return nil
		}
		existing, perr := strm.Parse(raw)
		if perr != nil {
			res.Conflicts++
			res.addDiagnostic(fmt.Sprintf("owned file is not a valid STRM: %s", rel))
			return nil
		}
		result, cerr := strm.Compare(existing, desired)
		if cerr != nil {
			res.Conflicts++
			res.addDiagnostic(fmt.Sprintf("owned file is not comparable: %s", rel))
			return nil
		}
		switch result.Decision {
		case strm.Unchanged:
			res.Skipped++
			if result.Anomaly != "" {
				res.addDiagnostic(fmt.Sprintf("%s: %s", rel, result.Anomaly))
			}
			if err := refreshOwnership(); err != nil {
				res.Errors++
				res.addDiagnostic(fmt.Sprintf("ownership mapping not refreshed: %s", rel))
			}
			return nil
		default:
			// A trusted owned target requires new content. In dry-run this is a
			// preview; otherwise the target is atomically replaced while
			// preserving the previous ownership mapping.
			if s.opts.DryRun {
				res.Updated++
				res.addDiagnostic(fmt.Sprintf("would update %s", rel))
				return nil
			}
			if err := s.opts.FS.Update(src.Output, rel, raw, desired.Bytes()); err != nil {
				res.Errors++
				res.addDiagnostic(fmt.Sprintf("update failed for %s", rel))
				return nil
			}
			res.Updated++
			if err := refreshOwnership(); err != nil {
				res.Errors++
				res.addDiagnostic(fmt.Sprintf("ownership mapping not refreshed: %s", rel))
			}
			return nil
		}
	}

	// Target does not exist. Plan or perform creation.
	if s.opts.DryRun {
		res.Created++
		res.addDiagnostic(fmt.Sprintf("would create %s", rel))
		return nil
	}
	if err := s.opts.FS.EnsureDir(src.Output, parentDir(rel)); err != nil {
		res.Errors++
		res.addDiagnostic(fmt.Sprintf("cannot create directory for %s", rel))
		return nil
	}
	if err := s.opts.FS.Publish(src.Output, rel, desired.Bytes()); err != nil {
		res.Errors++
		res.addDiagnostic(fmt.Sprintf("publish failed for %s", rel))
		return nil
	}
	res.Created++
	if s.store != nil {
		if err := refreshOwnership(); err != nil {
			// The file stays on disk but is not owned; a later run must treat
			// it as an unowned conflict.
			res.Errors++
			res.addDiagnostic(fmt.Sprintf("published but ownership not recorded: %s", rel))
			return nil
		}
		// afterOwnershipRecord is inert in release builds. The optional
		// strmgen_testhooks build can stop deterministically at exactly this
		// window (ownership durable, round not yet committed) for recovery
		// verification without a timing race.
		afterOwnershipRecord(rel)
	}
	return nil
}

// expectedTarget returns the local absolute path the configured source root
// implies for one relative path, and validates the relative path shape.
func expectedTarget(output, rel string) (string, bool) {
	if !validOwnedRelative(rel) {
		return "", false
	}
	return filepath.Join(output, filepath.FromSlash(rel)), true
}

// validOwnedRelative accepts only the generated STRM shape: clean forward-slash
// relative path segments with a .strm suffix.
func validOwnedRelative(rel string) bool {
	if rel == "" || len(rel) > pathmap.MaxRelativePathBytes || !strings.HasSuffix(rel, ".strm") {
		return false
	}
	for _, part := range strings.Split(rel, "/") {
		if part == "" || part == "." || part == ".." || strings.ContainsAny(part, "\\\x00") {
			return false
		}
	}
	return true
}

var errRootUnavailable = errors.New("source root became unreadable during confirmation")

// confirmAndCleanup stages missing observations for every unobserved owned
// target and then deletes rows whose already-committed evidence reaches the
// configured threshold. The durable counts only change in CommitRound.
func (s *Syncer) confirmAndCleanup(ctx context.Context, src Source, res *SourceResult) error {
	if s.store == nil {
		return nil
	}
	if !s.store.PreciseMappingAvailable() {
		res.addDiagnostic("legacy ownership lacks an exact remote mapping; cleanup preserved")
		if count, err := s.store.CountUnmappedMissing(ctx, src.Key); err == nil {
			res.Retained += int(count)
		}
		return nil
	}
	if err := s.confirmMissing(ctx, src, res); err != nil {
		return err
	}
	return s.cleanupCandidates(ctx, src, res)
}

// confirmMissing reads the current remote facts for every unobserved owned
// target. Present targets are cleared conservatively; missing targets are
// staged for this round only; an unreadable root or caller cancellation stops
// the source without staging anything further.
func (s *Syncer) confirmMissing(ctx context.Context, src Source, res *SourceResult) error {
	if s.confirmer == nil {
		return errors.New("no remote confirmer is configured")
	}
	var pending []string
	flush := func() error {
		if len(pending) == 0 {
			return nil
		}
		if err := s.store.AddPendingMissing(ctx, src.Key, pending); err != nil {
			return err
		}
		pending = pending[:0]
		return nil
	}
	after := ""
	for {
		batch, err := s.store.ListOwnershipAfter(ctx, src.Key, after, pendingBatchSize)
		if err != nil {
			return err
		}
		if len(batch) == 0 {
			break
		}
		for _, row := range batch {
			if err := ctx.Err(); err != nil {
				return err
			}
			after = row.RelativePath
			expected, ok := expectedTarget(src.Output, row.RelativePath)
			if !ok || row.AbsolutePath != expected {
				res.Retained++
				res.addDiagnostic("ownership record is outside the configured output boundary; preserved")
				continue
			}
			if row.RemotePath == "" {
				res.Retained++
				res.addDiagnostic("ownership record lacks an exact remote mapping; preserved")
				continue
			}
			observed, err := s.store.IsObserved(ctx, src.Key, row.RelativePath)
			if err != nil {
				return err
			}
			if observed {
				continue
			}
			result := s.confirmer.Confirm(ctx, src.Remote, row.RemotePath)
			switch result.Status {
			case openlist.AbsencePresent:
				if err := s.store.ClearMissing(ctx, src.Key, row.RelativePath); err != nil {
					return err
				}
				if err := s.store.MarkObserved(ctx, src.Key, row.RelativePath); err != nil {
					return err
				}
				if result.Reason == openlist.ReasonTargetTypeConflict {
					res.Retained++
					res.addDiagnostic(fmt.Sprintf("target type conflict preserved: %s", row.RelativePath))
				}
			case openlist.AbsenceMissing:
				pending = append(pending, row.RelativePath)
				res.ConfirmedMissing++
				if len(pending) >= pendingBatchSize {
					if err := flush(); err != nil {
						return err
					}
				}
			default:
				switch result.Reason {
				case openlist.ReasonCancelled:
					if err := ctx.Err(); err != nil {
						return err
					}
					return errors.New("confirmation cancelled")
				case openlist.ReasonRootUnavailable:
					return errRootUnavailable
				default:
					res.Retained++
					res.addDiagnostic(fmt.Sprintf("unconfirmed ownership preserved: %s", row.RelativePath))
				}
			}
		}
	}
	return flush()
}

// cleanupCandidates consumes the already-committed evidence. Before every
// unlink it re-reads the current source root and exact target, verifies the
// ownership boundary and local type, then removes the target and its ownership
// row. Any local failure stops the remaining cleanup; already deleted targets
// are reported and the round is not committed.
func (s *Syncer) cleanupCandidates(ctx context.Context, src Source, res *SourceResult) error {
	after := ""
	for {
		batch, err := s.store.ListEligibleAfter(ctx, src.Key, after, s.opts.MissingConfirmations, pendingBatchSize)
		if err != nil {
			return err
		}
		if len(batch) == 0 {
			break
		}
		for _, row := range batch {
			if err := ctx.Err(); err != nil {
				return err
			}
			after = row.RelativePath
			if err := s.deleteCandidate(ctx, src, row, res); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *Syncer) deleteCandidate(ctx context.Context, src Source, row storage.Ownership, res *SourceResult) error {
	expected, ok := expectedTarget(src.Output, row.RelativePath)
	if !ok || row.AbsolutePath != expected {
		res.Retained++
		res.addDiagnostic("cleanup candidate is outside the configured output boundary; preserved")
		return nil
	}
	observed, err := s.store.IsObserved(ctx, src.Key, row.RelativePath)
	if err != nil {
		return err
	}
	if observed {
		return nil
	}
	result := s.confirmer.Confirm(ctx, src.Remote, row.RemotePath)
	switch result.Status {
	case openlist.AbsencePresent:
		if err := s.store.ClearMissing(ctx, src.Key, row.RelativePath); err != nil {
			return err
		}
		if err := s.store.MarkObserved(ctx, src.Key, row.RelativePath); err != nil {
			return err
		}
		if result.Reason == openlist.ReasonTargetTypeConflict {
			res.Retained++
			res.addDiagnostic(fmt.Sprintf("target type conflict preserved: %s", row.RelativePath))
		}
		return nil
	case openlist.AbsenceMissing:
	default:
		switch result.Reason {
		case openlist.ReasonCancelled:
			if err := ctx.Err(); err != nil {
				return err
			}
			return errors.New("confirmation cancelled")
		case openlist.ReasonRootUnavailable:
			return errRootUnavailable
		default:
			res.Retained++
			res.addDiagnostic(fmt.Sprintf("cleanup candidate could not be reconfirmed; preserved: %s", row.RelativePath))
			return nil
		}
	}
	if err := s.opts.FS.InspectDeletable(ctx, src.Output, row.RelativePath); err != nil {
		if errors.Is(err, ErrStagingPresent) || errors.Is(err, ErrNotDeletable) {
			res.Retained++
			res.addDiagnostic(fmt.Sprintf("local target is not safely deletable; preserved: %s", row.RelativePath))
			return nil
		}
		return err
	}
	outcome, err := s.opts.FS.DeleteOwned(ctx, src.Output, row.RelativePath)
	if outcome.Removed {
		res.Deleted++
	}
	if err != nil {
		return err
	}
	if err := s.store.RemoveOwnership(ctx, src.Key, row.RelativePath); err != nil {
		return err
	}
	if outcome.AlreadyAbsent {
		res.Reclaimed++
	} else {
		res.addDiagnostic(fmt.Sprintf("deleted %s", row.RelativePath))
	}
	return nil
}

// previewCleanup lists the current committed candidates without changing any
// state. A candidate is reported only when the current source root, exact
// target, ownership boundary and local type checks all still hold.
func (s *Syncer) previewCleanup(ctx context.Context, src Source, res *SourceResult) error {
	if s.store == nil {
		return nil
	}
	if !s.store.PreciseMappingAvailable() {
		res.addDiagnostic("legacy ownership lacks an exact remote mapping; no cleanup candidate previewed")
		if count, err := s.store.CountUnmappedMissing(ctx, src.Key); err == nil {
			res.Retained += int(count)
		}
		return nil
	}
	if s.confirmer == nil {
		return errors.New("no remote confirmer is configured")
	}
	after := ""
	for {
		batch, err := s.store.ListEligibleAfter(ctx, src.Key, after, s.opts.MissingConfirmations, pendingBatchSize)
		if err != nil {
			return err
		}
		if len(batch) == 0 {
			break
		}
		for _, row := range batch {
			if err := ctx.Err(); err != nil {
				return err
			}
			after = row.RelativePath
			expected, ok := expectedTarget(src.Output, row.RelativePath)
			if !ok || row.AbsolutePath != expected {
				res.Retained++
				res.addDiagnostic("cleanup candidate is outside the configured output boundary; preserved")
				continue
			}
			observed, err := s.store.IsObserved(ctx, src.Key, row.RelativePath)
			if err != nil {
				return err
			}
			if observed {
				continue
			}
			result := s.confirmer.Confirm(ctx, src.Remote, row.RemotePath)
			switch result.Status {
			case openlist.AbsencePresent:
				if result.Reason == openlist.ReasonTargetTypeConflict {
					res.addDiagnostic(fmt.Sprintf("target type conflict preserved: %s", row.RelativePath))
				}
				continue
			case openlist.AbsenceMissing:
			default:
				switch result.Reason {
				case openlist.ReasonCancelled:
					if err := ctx.Err(); err != nil {
						return err
					}
					return errors.New("confirmation cancelled")
				case openlist.ReasonRootUnavailable:
					return errRootUnavailable
				default:
					res.Retained++
					res.addDiagnostic(fmt.Sprintf("preview candidate could not be reconfirmed; preserved: %s", row.RelativePath))
					continue
				}
			}
			if err := s.opts.FS.InspectDeletable(ctx, src.Output, row.RelativePath); err != nil {
				if errors.Is(err, ErrStagingPresent) || errors.Is(err, ErrNotDeletable) {
					res.Retained++
					res.addDiagnostic(fmt.Sprintf("local target is not safely deletable; preserved: %s", row.RelativePath))
					continue
				}
				return err
			}
			res.Candidates++
			res.addDiagnostic(fmt.Sprintf("would delete %s", row.RelativePath))
		}
	}
	return nil
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
	onDir   func(remoteDir, relDir string, entries []openlist.Entry) error
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
			if err := w.onDir(r.dir, r.relDir, r.entries); err != nil {
				if firstErr == nil {
					firstErr = err
				}
				stopped = true
				continue
			}
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
						firstErr = errors.New("remote directory path exceeds length limit")
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
