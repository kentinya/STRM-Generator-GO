package openlist

import (
	"context"
	"errors"
	"strings"
	"unicode/utf8"
)

// AbsenceStatus is the outcome class of one remote absence confirmation. The
// zero value is deliberately Unknown: a result that is ignored, missed or
// re-used by mistake can never be read as proof of absence.
type AbsenceStatus int

const (
	// AbsenceUnknown means the current remote fact could not be established.
	// Every error, rejection, ambiguity and limit resolves here.
	AbsenceUnknown AbsenceStatus = iota
	// AbsencePresent means the exact target name exists under the confirmed
	// parent directory, whatever its type.
	AbsencePresent
	// AbsenceConfirmed means a fresh, complete listing of an accessible parent
	// that is anchored to a real storage object proved the exact name absent.
	AbsenceConfirmed
)

// String renders the status without exposing request details.
func (s AbsenceStatus) String() string {
	switch s {
	case AbsencePresent:
		return "present"
	case AbsenceConfirmed:
		return "confirmed-absent"
	default:
		return "unknown"
	}
}

// AbsenceReason is a bounded, non-secret explanation carried by an
// AbsenceResult. It never contains a remote message, token, cookie, signed URL
// or full private path.
type AbsenceReason string

const (
	ReasonNone                AbsenceReason = ""
	ReasonInvalidInput        AbsenceReason = "source root or target is not a clean descendant path"
	ReasonRootIsTarget        AbsenceReason = "target is the configured source root"
	ReasonRootUnavailable     AbsenceReason = "source root is not a readable directory"
	ReasonRootNotRealStorage  AbsenceReason = "source root is not anchored to a real storage object"
	ReasonDirectoryUnlisted   AbsenceReason = "directory could not be freshly listed"
	ReasonVirtualViewObserved AbsenceReason = "path is a virtual mount view, not a real directory"
	ReasonNotADirectory       AbsenceReason = "path component exists but is not a directory"
	ReasonParentUndetermined  AbsenceReason = "target parent is not an established real directory"
	ReasonDepthLimit          AbsenceReason = "remote path exceeds the supported confirmation depth"
	ReasonRequestLimit        AbsenceReason = "confirmation request limit exceeded"
	ReasonTargetPresent       AbsenceReason = "exact target name exists in the listed parent"
	ReasonTargetAbsent        AbsenceReason = "exact target name is absent from a fresh complete listing"
	ReasonAncestorAbsent      AbsenceReason = "an ancestor name is absent from a fresh complete listing"
	ReasonPermissionFiltered  AbsenceReason = "requesting identity is not permitted to see part of the path"
	ReasonTargetUnresolved    AbsenceReason = "listing and direct read disagree about the target"
	ReasonCancelled           AbsenceReason = "confirmation was cancelled"
)

// AbsenceResult is the outcome of exactly one confirmation call. It is a
// transient observation, never a durable state, and must not be cached across
// rounds.
type AbsenceResult struct {
	Status AbsenceStatus
	Reason AbsenceReason
	// ConfirmedParent is the parent path whose fresh complete listing produced
	// the result. Empty unless the status is Present or Confirmed.
	ConfirmedParent string
	// ConfirmedName is the final segment the result refers to. Empty unless the
	// status is Present or Confirmed.
	ConfirmedName string
	// Chain lists the directories verified from the source root down to
	// ConfirmedParent, inclusive. Bounded by MaxAbsenceDepth.
	Chain []string
}

// FreshLister is the remote capability an absence confirmation needs: a
// listing that forces a server-side refresh, plus a single-object read used
// only to test whether a path is anchored to a real storage object.
type FreshLister interface {
	Get(ctx context.Context, remotePath string) (*Entry, error)
	ListFresh(ctx context.Context, remotePath string) ([]Entry, error)
}

// Confirmation limits. They bound one call in depth, requests and memory
// independently of how deep or wide the configured source tree is.
const (
	// MaxAbsenceDepth bounds the number of segments below the source root that
	// one confirmation may verify.
	MaxAbsenceDepth = 64
	// MaxAbsenceRequests bounds the total remote requests one confirmation may
	// issue: one anchor read per verified directory plus one fresh listing per
	// verified directory and the final parent.
	MaxAbsenceRequests = 2*MaxAbsenceDepth + 4
)

// ErrFreshListingUnsupported reports that a client cannot force a refresh and
// therefore must never be used as an absence source.
var ErrFreshListingUnsupported = errors.New("client cannot force a fresh remote listing")

// Confirmer proves current remote existence for one exact path under a source
// root. It holds no per-call or cross-call state: every Confirm re-reads the
// remote facts, so a previous "absent" result can never be reused.
type Confirmer struct {
	lister FreshLister
}

// NewConfirmer builds a Confirmer over the given remote client.
func NewConfirmer(lister FreshLister) *Confirmer {
	return &Confirmer{lister: lister}
}

// Confirm decides whether targetPath currently exists under sourceRoot.
//
// AbsenceConfirmed is returned only when every one of these holds:
//   - the source root answers with a real directory object (not a virtual
//     mount view, which OpenList can serve while the driver is unreachable);
//   - every parent directory down to the target's parent was freshly listed in
//     full with a consistent total;
//   - each intermediate name was found and is a directory;
//   - the exact final name (case-sensitive) is absent from that fresh listing.
//
// Every other outcome - transport error, HTTP or application error, permission
// rejection, truncated or contradictory pagination, a component that is not a
// directory, a virtual mount view, a depth or request limit - is
// AbsenceUnknown. A target that exists as any type is AbsencePresent.
func (c *Confirmer) Confirm(ctx context.Context, sourceRoot, targetPath string) AbsenceResult {
	if c == nil || c.lister == nil {
		return unknownResult(ReasonRootUnavailable)
	}
	root, segments, reason := splitDescendant(sourceRoot, targetPath)
	if reason != ReasonNone {
		return unknownResult(reason)
	}
	budget := MaxAbsenceRequests
	if err := ctx.Err(); err != nil {
		return unknownResult(ReasonCancelled)
	}

	// Anchor the source root to a real storage object first. OpenList answers a
	// path that only exists because a nested mount lives below it with a virtual
	// directory whose `provider` is "unknown"; the list of such a path can
	// succeed while the underlying driver is down, so it can never anchor a
	// negative conclusion.
	reason = c.verifyRealDirectory(ctx, root, &budget)
	if reason != ReasonNone {
		return unknownResult(reason)
	}

	chain := make([]string, 0, len(segments))
	parent := root
	for _, segment := range segments[:len(segments)-1] {
		entries, reason := c.freshEntries(ctx, parent, &budget)
		if reason != ReasonNone {
			return unknownResult(reason)
		}
		entry, ok := exactEntry(entries, segment)
		if !ok {
			// A fresh, complete listing of an established real directory that
			// omits the name proves the whole remaining path cannot exist. This
			// is the ancestor-chain case where a parent directory itself was
			// removed: no further request is needed and none may be attempted on
			// the missing path, which would only turn a driver error into what
			// looks like more evidence.
			chain = append(chain, parent)
			return AbsenceResult{
				Status:          AbsenceConfirmed,
				Reason:          ReasonAncestorAbsent,
				ConfirmedParent: parent,
				ConfirmedName:   segment,
				Chain:           chain,
			}
		}
		if !entry.IsDir {
			return unknownResult(ReasonNotADirectory)
		}
		chain = append(chain, parent)
		parent = joinRemote(parent, segment)
		// Each descending step must itself be a real directory object, so a
		// nested mount view can never become an intermediate confirmation
		// anchor.
		if reason := c.verifyRealDirectory(ctx, parent, &budget); reason != ReasonNone {
			return unknownResult(reason)
		}
	}

	entries, reason := c.freshEntries(ctx, parent, &budget)
	if reason != ReasonNone {
		return unknownResult(reason)
	}
	final := segments[len(segments)-1]
	chain = append(chain, parent)
	if _, ok := exactEntry(entries, final); ok {
		return AbsenceResult{
			Status:          AbsencePresent,
			Reason:          ReasonTargetPresent,
			ConfirmedParent: parent,
			ConfirmedName:   final,
			Chain:           chain,
		}
	}
	// The name is missing from a fresh complete listing, but OpenList also drops
	// entries the requesting identity may not see (a metadata `hide` rule), and
	// such an entry is indistinguishable from a deleted one in a listing. One
	// direct object read separates the two cases without ever being sufficient on
	// its own: a structured permission refusal means the name exists but is
	// filtered, a successful read contradicts the listing, and any other failure
	// leaves the listing as the only evidence, which is exactly what established
	// the absence in the first place.
	if reason := c.probeFilteredTarget(ctx, parent, final, &budget); reason != ReasonNone {
		return unknownResult(reason)
	}
	return AbsenceResult{
		Status:          AbsenceConfirmed,
		Reason:          ReasonTargetAbsent,
		ConfirmedParent: parent,
		ConfirmedName:   final,
		Chain:           chain,
	}
}

// probeFilteredTarget guards a negative listing against visibility filtering.
//
// It returns ReasonPermissionFiltered when OpenList structurally refuses the
// direct read (the name exists but this identity may not see it),
// ReasonTargetUnresolved when the read succeeds (contradicting the listing) or
// when it cannot be performed at all, and ReasonNone only when the read failed
// in a way consistent with an absent object.
func (c *Confirmer) probeFilteredTarget(ctx context.Context, parent, name string, budget *int) AbsenceReason {
	if err := ctx.Err(); err != nil {
		return ReasonCancelled
	}
	if reason := takeRequest(budget); reason != ReasonNone {
		return reason
	}
	entry, err := c.lister.Get(ctx, joinRemote(parent, name))
	if err == nil && entry != nil {
		return ReasonTargetUnresolved
	}
	if IsPermissionDenied(err) {
		return ReasonPermissionFiltered
	}
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		// The service answered with an application-level failure. The pinned
		// build uses the same code for a missing object, a driver error and a
		// virtual mount view, so the failure adds no new evidence: the fresh
		// listing remains the deciding fact.
		return ReasonNone
	}
	// A transport error, timeout, cancellation or malformed response means the
	// probe could not be performed, so the negative conclusion is withheld.
	return ReasonTargetUnresolved
}

func unknownResult(reason AbsenceReason) AbsenceResult {
	return AbsenceResult{Status: AbsenceUnknown, Reason: reason}
}

// takeRequest consumes one unit of the per-call request budget.
func takeRequest(budget *int) AbsenceReason {
	if *budget <= 0 {
		return ReasonRequestLimit
	}
	*budget--
	return ReasonNone
}

// verifyRealDirectory requires path to answer a Get with a directory object
// backed by a real storage. OpenList reports "unknown" as the provider for a
// directory that only exists as the parent of a nested mount, and can serve
// that directory successfully while its own driver is unreachable.
func (c *Confirmer) verifyRealDirectory(ctx context.Context, path string, budget *int) AbsenceReason {
	if err := ctx.Err(); err != nil {
		return ReasonCancelled
	}
	if reason := takeRequest(budget); reason != ReasonNone {
		return reason
	}
	entry, err := c.lister.Get(ctx, path)
	if err != nil || entry == nil {
		return ReasonRootUnavailable
	}
	if entry.Name == "" || !entry.IsDir {
		return ReasonNotADirectory
	}
	if entry.Virtual {
		return ReasonVirtualViewObserved
	}
	return ReasonNone
}

// freshEntries lists one directory with an explicit refresh. A failed refresh
// is an error; it never falls back to a cached List, because a cached page can
// predate the change being confirmed.
func (c *Confirmer) freshEntries(ctx context.Context, dir string, budget *int) ([]Entry, AbsenceReason) {
	if err := ctx.Err(); err != nil {
		return nil, ReasonCancelled
	}
	if reason := takeRequest(budget); reason != ReasonNone {
		return nil, reason
	}
	entries, err := c.lister.ListFresh(ctx, dir)
	if err != nil {
		return nil, ReasonDirectoryUnlisted
	}
	return entries, ReasonNone
}

func exactEntry(entries []Entry, name string) (Entry, bool) {
	for i := range entries {
		if entries[i].Name == name {
			return entries[i], true
		}
	}
	return Entry{}, false
}

func joinRemote(dir, name string) string {
	if dir == "/" {
		return "/" + name
	}
	return dir + "/" + name
}

// splitDescendant validates that targetPath is a strict descendant of
// sourceRoot and returns the root plus the remaining path segments.
//
// The inputs are never normalised: a spelling that is not already a clean,
// absolute path is rejected instead of being silently cleaned into a different
// path than the caller asked about. Accepting a dirty spelling would let a
// caller confirm absence for a path it did not actually name.
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
		if !strings.HasPrefix(target, "/") {
			return "", nil, ReasonInvalidInput
		}
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
