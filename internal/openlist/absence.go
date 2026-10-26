package openlist

import (
	"context"
	"errors"
	"path"
	"strings"
	"unicode/utf8"

	"strm-generator-go/internal/pathmap"
)

// AbsenceStatus is the outcome class of one remote target read. The zero value
// is deliberately Unknown so an ignored, missed or mis-used result can never be
// read as a missing observation.
type AbsenceStatus int

const (
	// AbsenceUnknown means the current remote fact was not established: the
	// call was cancelled, the input was rejected, the configured source root
	// did not read as a directory, or no reader is configured. A caller must
	// preserve the source and add no missing observation.
	AbsenceUnknown AbsenceStatus = iota
	// AbsencePresent means the exact target path currently reads as an existing
	// object. A directory target or any other readable type is present too: a
	// type conflict is reported, never turned into a missing observation.
	AbsencePresent
	// AbsenceMissing means the source root read as a directory but the exact
	// target path was not read in this call. It is a business observation for
	// this call only, not a proof that the object does not exist: OpenList
	// answers a genuinely missing object and several failure modes identically,
	// and the accepted rule records all of them the same way.
	AbsenceMissing
)

// String renders the status without exposing request details.
func (s AbsenceStatus) String() string {
	switch s {
	case AbsencePresent:
		return "present"
	case AbsenceMissing:
		return "missing"
	default:
		return "unknown"
	}
}

// AbsenceReason is a bounded, non-secret explanation carried by an
// AbsenceResult. It never contains a remote message, token, cookie, signed URL
// or full private path.
type AbsenceReason string

const (
	ReasonNone               AbsenceReason = ""
	ReasonInvalidInput       AbsenceReason = "source root or target is not a clean descendant path"
	ReasonRootIsTarget       AbsenceReason = "target is the configured source root"
	ReasonDepthLimit         AbsenceReason = "remote path exceeds the supported confirmation depth"
	ReasonDependencyMissing  AbsenceReason = "no remote reader is configured"
	ReasonCancelled          AbsenceReason = "confirmation was cancelled by the caller"
	ReasonRootUnavailable    AbsenceReason = "source root could not be read as a directory"
	ReasonTargetPresent      AbsenceReason = "target was read as an existing object"
	ReasonTargetTypeConflict AbsenceReason = "target was read but is a directory, not a file"
	ReasonTargetUnread       AbsenceReason = "target was not read in this call"
	ReasonTargetInvalid      AbsenceReason = "target response was not the requested object"
)

// AbsenceResult is the outcome of exactly one confirmation call. It is a
// transient observation, never a durable state, and must not be cached across
// rounds.
type AbsenceResult struct {
	Status AbsenceStatus
	Reason AbsenceReason
}

// Getter is the only remote capability a confirmation needs: one exact-object
// read. A confirmation never lists a directory and never walks ancestors.
type Getter interface {
	Get(ctx context.Context, remotePath string) (*Entry, error)
}

const (
	// MaxAbsenceDepth bounds the number of segments below the source root that
	// one confirmation may accept.
	MaxAbsenceDepth = 64
	// MaxAbsenceReads bounds the remote reads one valid confirmation issues:
	// one for the source root and one for the exact target.
	MaxAbsenceReads = 2
)

// errCallerCancelled distinguishes a caller cancellation or deadline from a
// request failure. The caller's context ending aborts the whole call, so it may
// never be classified as a missing observation.
var errCallerCancelled = errors.New("confirmation context ended")

// Confirmer reads the current remote facts for one exact path under a source
// root. It holds no per-call or cross-call state: every Confirm re-reads the
// remote facts, so a previous result can never be reused.
type Confirmer struct {
	getter Getter
	media  *pathmap.MediaSet
}

// NewConfirmer builds a Confirmer over the given remote reader.
func NewConfirmer(getter Getter) *Confirmer {
	return &Confirmer{getter: getter}
}

// WithMediaSet replaces the extension set used by IsMedia. A nil set keeps the
// default list, so an unconfigured caller behaviour does not change.
func (c *Confirmer) WithMediaSet(set *pathmap.MediaSet) *Confirmer {
	c.media = set
	return c
}

// IsMedia reports whether the last extension of one remote path is currently
// generated. It is pure and only looks at the file name suffix, so the cleanup
// rule can ask the same question the directory walk asks without a remote read.
// A nil receiver keeps the default list, matching an unconfigured instance.
func (c *Confirmer) IsMedia(remotePath string) bool {
	if c == nil {
		return pathmap.IsMedia(path.Base(remotePath))
	}
	return c.media.IsMedia(path.Base(remotePath))
}

// Confirm reads the current remote facts for one exact target path under a
// configured source root and classifies the result for the caller's cleanup
// rule (requirements: simplified deletion decision).
//
// The decision is deliberately two exact-object reads and no listing:
//
//  1. Get(sourceRoot). A failure, an unusable object or a non-directory result
//     is AbsenceUnknown / ReasonRootUnavailable: the caller preserves the
//     source STRM and adds no missing observation. No target read is issued.
//  2. Get(targetPath). A successfully decoded object is AbsencePresent; a
//     directory is AbsencePresent with ReasonTargetTypeConflict. Any error or
//     unusable object is AbsenceMissing: one missing observation for this call,
//     regardless of HTTP status, application code, transport failure, a single
//     request timeout or an unparseable response.
//
// A cancellation or deadline of the caller's own context is never a missing
// observation: it is checked before and after every request and wins over the
// request result. A single HTTP request timeout of the client is different: its
// parent context is still valid, so it is classified by stage (root
// unavailable, target missing).
//
// Confirm does not claim that a missing observation proves the object does not
// exist. It reports what the configured rule asks for: the root read as a
// directory and the exact target was not read.
func (c *Confirmer) Confirm(ctx context.Context, sourceRoot, targetPath string) AbsenceResult {
	if c == nil || c.getter == nil {
		return unknownResult(ReasonDependencyMissing)
	}
	if ctx == nil {
		return unknownResult(ReasonCancelled)
	}
	root, segments, reason := splitDescendant(sourceRoot, targetPath)
	if reason != ReasonNone {
		return unknownResult(reason)
	}
	if reason := CheckSourceRoot(ctx, c.getter, root); reason != ReasonNone {
		return unknownResult(reason)
	}

	target := joinSegments(root, segments)
	targetEntry, err := c.read(ctx, target)
	if errors.Is(err, errCallerCancelled) {
		return unknownResult(ReasonCancelled)
	}
	name := segments[len(segments)-1]
	switch {
	case err != nil || targetEntry == nil || targetEntry.Name == "":
		return missingResult(ReasonTargetUnread)
	case targetEntry.Name != name:
		// The response is about a different object, so the exact target was not
		// read. This is an unusable response, not evidence about the requested
		// name.
		return missingResult(ReasonTargetInvalid)
	case targetEntry.IsDir:
		return presentResult(ReasonTargetTypeConflict)
	default:
		return presentResult(ReasonTargetPresent)
	}
}

// read performs one exact-object read and enforces the caller-cancellation
// rule on both sides of the request.
func (c *Confirmer) read(ctx context.Context, path string) (*Entry, error) {
	if ctx.Err() != nil {
		return nil, errCallerCancelled
	}
	entry, err := c.getter.Get(ctx, path)
	if ctx.Err() != nil {
		return nil, errCallerCancelled
	}
	return entry, err
}

// CheckSourceRoot reads the configured source root and reports whether this
// call may continue. It is the single definition of the source-root gate used
// both by Confirmer and by the directory reconciler, so a root that is removed,
// unreadable, malformed or not a directory can never be interpreted as an
// empty source.
//
// ReasonNone means the root was read as a directory. Every other reason is a
// preserve decision for the caller: the source keeps its STRM and missing
// counts. ReasonCancelled reports that the caller's own context ended.
func CheckSourceRoot(ctx context.Context, getter Getter, sourceRoot string) AbsenceReason {
	if getter == nil {
		return ReasonDependencyMissing
	}
	if ctx == nil || ctx.Err() != nil {
		return ReasonCancelled
	}
	root, ok := cleanRemote(sourceRoot)
	if !ok {
		return ReasonInvalidInput
	}
	entry, err := getter.Get(ctx, root)
	if ctx.Err() != nil {
		return ReasonCancelled
	}
	// A Get object is only usable when it carries a name. Client.Get already
	// rejects a response without one, but a reader supplied by a caller must not
	// be able to turn a zero object into a readable directory.
	if err != nil || entry == nil || entry.Name == "" || !entry.IsDir {
		return ReasonRootUnavailable
	}
	return ReasonNone
}

func unknownResult(reason AbsenceReason) AbsenceResult {
	return AbsenceResult{Status: AbsenceUnknown, Reason: reason}
}

func presentResult(reason AbsenceReason) AbsenceResult {
	return AbsenceResult{Status: AbsencePresent, Reason: reason}
}

func missingResult(reason AbsenceReason) AbsenceResult {
	return AbsenceResult{Status: AbsenceMissing, Reason: reason}
}

func joinSegments(root string, segments []string) string {
	if root == "/" {
		return "/" + strings.Join(segments, "/")
	}
	return root + "/" + strings.Join(segments, "/")
}

// splitDescendant validates that targetPath is a strict descendant of
// sourceRoot and returns the root plus the remaining path segments.
//
// The inputs are never normalised: a spelling that is not already a clean,
// absolute path is rejected instead of being silently cleaned into a different
// path than the caller asked about. Accepting a dirty spelling would let a
// caller confirm a path it did not actually name.
func splitDescendant(sourceRoot, targetPath string) (string, []string, AbsenceReason) {
	root, ok := cleanRemote(sourceRoot)
	if !ok {
		return "", nil, ReasonInvalidInput
	}
	target, ok := cleanRemote(targetPath)
	if !ok {
		return "", nil, ReasonInvalidInput
	}
	if target == root {
		return "", nil, ReasonRootIsTarget
	}
	var rest string
	if root == "/" {
		rest = strings.TrimPrefix(target, "/")
	} else {
		if !strings.HasPrefix(target, root+"/") {
			return "", nil, ReasonInvalidInput
		}
		rest = strings.TrimPrefix(target, root+"/")
	}
	if rest == "" {
		return "", nil, ReasonRootIsTarget
	}
	segments := strings.Split(rest, "/")
	if len(segments) > MaxAbsenceDepth {
		return "", nil, ReasonDepthLimit
	}
	for _, segment := range segments {
		if !validSegment(segment) {
			return "", nil, ReasonInvalidInput
		}
	}
	return root, segments, ReasonNone
}

// cleanRemote accepts only an already-clean absolute remote path. A trailing
// slash, empty segment, dot segment, backslash or control character is
// rejected rather than normalised away.
func cleanRemote(raw string) (string, bool) {
	if raw == "" || len(raw) > 4096 || !utf8.ValidString(raw) {
		return "", false
	}
	if !strings.HasPrefix(raw, "/") {
		return "", false
	}
	if raw == "/" {
		return "/", true
	}
	if strings.HasSuffix(raw, "/") {
		return "", false
	}
	for _, segment := range strings.Split(strings.TrimPrefix(raw, "/"), "/") {
		if !validSegment(segment) {
			return "", false
		}
	}
	return raw, true
}

func validSegment(segment string) bool {
	switch segment {
	case "", ".", "..":
		return false
	}
	if !utf8.ValidString(segment) || len(segment) > 255 {
		return false
	}
	if strings.ContainsAny(segment, "/\\") {
		return false
	}
	for _, r := range segment {
		if r < 0x20 || r == 0x7f {
			return false
		}
	}
	return true
}
