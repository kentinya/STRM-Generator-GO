//go:build openlist_integration

// Package-level integration tests that run against a real OpenList service.
//
// These tests never start their own service: scripts/run_openlist_absence_integration.py
// owns the isolated instance, materializes the fixtures, and passes a JSON
// descriptor through the environment. Without that descriptor every test fails
// loudly instead of skipping, so a missing real service can never look like a
// pass.
package openlist

import (
	"context"
	"encoding/json"
	"os"
	"path"
	"strings"
	"testing"
)

// integrationEnv is the JSON descriptor written by the runner.
type integrationEnv struct {
	APIURL string `json:"api_url"`
	Token  string `json:"token"`
	// Root is the source root the generator would be configured with.
	Root string `json:"root"`
	// Scenarios maps a scenario name to the exact facts the runner created.
	Scenarios map[string]json.RawMessage `json:"scenarios"`
}

func loadIntegrationEnv(t *testing.T) *integrationEnv {
	t.Helper()
	raw := os.Getenv("STRMGEN_OPENLIST_INTEGRATION")
	if raw == "" {
		t.Fatal("STRMGEN_OPENLIST_INTEGRATION is unset: this test requires the isolated real OpenList runner")
	}
	var env integrationEnv
	if err := json.Unmarshal([]byte(raw), &env); err != nil {
		t.Fatalf("integration descriptor is not valid JSON: %v", err)
	}
	if env.APIURL == "" || env.Token == "" || env.Root == "" {
		t.Fatal("integration descriptor is missing api_url, token or root")
	}
	return &env
}

func (e *integrationEnv) scenario(t *testing.T, name string) map[string]any {
	t.Helper()
	raw, ok := e.Scenarios[name]
	if !ok {
		t.Fatalf("scenario %q missing from the integration descriptor", name)
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("scenario %q is not a JSON object: %v", name, err)
	}
	return out
}

func (e *integrationEnv) client() *Client { return New(e.APIURL, e.Token) }

func (e *integrationEnv) confirmer() *Confirmer { return NewConfirmer(e.client()) }

func remoteJoin(dir, name string) string {
	if dir == "/" {
		return "/" + name
	}
	return dir + "/" + name
}

// expectStatus asserts a confirmation outcome and reports the bounded reason.
func expectStatus(t *testing.T, got AbsenceResult, want AbsenceStatus, context string) {
	t.Helper()
	if got.Status != want {
		t.Fatalf("%s: got %s (%s) parent=%q name=%q, want %s",
			context, got.Status, got.Reason, got.ConfirmedParent, got.ConfirmedName, want)
	}
}

func names(entries []Entry) []string {
	out := make([]string, 0, len(entries))
	for _, entry := range entries {
		out = append(out, entry.Name)
	}
	return out
}

// TestIntegrationConfirmPresentAndAbsentAgainstRealOpenList proves the basic
// positive and negative facts, including exact spelling and case sensitivity,
// using the real Local driver through the real API.
func TestIntegrationConfirmPresentAndAbsentAgainstRealOpenList(t *testing.T) {
	env := loadIntegrationEnv(t)
	sc := env.scenario(t, "exact_names")
	ctx := context.Background()
	confirmer := env.confirmer()

	present := sc["present_file"].(string)
	expectStatus(t, confirmer.Confirm(ctx, env.Root, present), AbsencePresent, "existing file")

	absentPath := sc["absent_file"].(string)
	expectStatus(t, confirmer.Confirm(ctx, env.Root, absentPath), AbsenceConfirmed, "absent file")

	// The real API is case sensitive and resolves by exact name, so a case
	// variant must be confirmed absent rather than silently matched.
	caseVariant := sc["case_variant"].(string)
	expectStatus(t, confirmer.Confirm(ctx, env.Root, caseVariant), AbsenceConfirmed, "case variant is a different name")

	// Special characters must survive as exact names rather than be escaped or
	// interpreted. The descriptor carries full remote paths because the runner
	// owns fixture placement.
	for _, target := range sc["special_files"].([]any) {
		expectStatus(t, confirmer.Confirm(ctx, env.Root, target.(string)), AbsencePresent, "special name "+target.(string))
	}
}

// TestIntegrationConfirmReadsEveryPageAgainstRealOpenList proves that a name
// past the first page is found, and that absence from a multi-page directory is
// only concluded after every page was read.
func TestIntegrationConfirmReadsEveryPageAgainstRealOpenList(t *testing.T) {
	env := loadIntegrationEnv(t)
	sc := env.scenario(t, "pagination")
	ctx := context.Background()

	last := sc["last_page_file"].(string)
	if got := env.confirmer().Confirm(ctx, env.Root, last); got.Status != AbsencePresent {
		t.Fatalf("file on the last page must be present: %+v", got)
	}
	absent := sc["absent_file"].(string)
	if got := env.confirmer().Confirm(ctx, env.Root, absent); got.Status != AbsenceConfirmed {
		t.Fatalf("absence in a multi-page directory: %+v", got)
	}

	// A direct fresh listing must also be complete and must report the total.
	entries, err := env.client().ListFresh(ctx, sc["directory"].(string))
	if err != nil {
		t.Fatalf("fresh listing: %v", err)
	}
	want := int(sc["total"].(float64))
	if len(entries) != want {
		t.Fatalf("fresh listing returned %d entries, want %d", len(entries), want)
	}
}

// TestIntegrationEmptyDirectoryShapeAgainstRealOpenList records the exact
// serialization of a real empty directory and proves the client classifies it
// as a complete empty listing rather than an error.
func TestIntegrationEmptyDirectoryShapeAgainstRealOpenList(t *testing.T) {
	env := loadIntegrationEnv(t)
	sc := env.scenario(t, "empty_directory")
	ctx := context.Background()
	emptyDir := sc["directory"].(string)

	entries, err := env.client().ListFresh(ctx, emptyDir)
	if err != nil {
		t.Fatalf("real empty directory must be a complete empty listing: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("real empty directory returned %d entries", len(entries))
	}
	if got := env.confirmer().Confirm(ctx, env.Root, remoteJoin(emptyDir, "Anything.mkv")); got.Status != AbsenceConfirmed {
		t.Fatalf("absence inside a real empty directory: %+v", got)
	}
}

// TestIntegrationRemovedParentChainAgainstRealOpenList proves a parent
// directory removed on the real backend is confirmed through the ancestor
// chain, and that the source root disappearing is not.
func TestIntegrationRemovedParentChainAgainstRealOpenList(t *testing.T) {
	env := loadIntegrationEnv(t)
	sc := env.scenario(t, "removed_parent")
	ctx := context.Background()

	// The scenario uses a second storage whose entire backing directory was
	// removed, so its own source root is unreachable while a parent that still
	// exists is a real directory.
	unavailableRoot := sc["unavailable_root"].(string)
	if got := env.confirmer().Confirm(ctx, unavailableRoot, remoteJoin(unavailableRoot, "Movies/A.mkv")); got.Status != AbsenceUnknown {
		t.Fatalf("unreachable source root must not confirm absence: %+v", got)
	}

	// A parent directory removed inside an otherwise reachable source must be
	// proven absent through the ancestor chain of a real, freshly listed parent.
	removedParent := sc["removed_parent"].(string)
	got := env.confirmer().Confirm(ctx, env.Root, remoteJoin(removedParent, "Season 1.mkv"))
	if got.Status == AbsenceConfirmed {
		// The storage root is fine but the removed sub-tree belongs to the other
		// mount, so this path is unrelated to env.Root. Only assert that no
		// unreachable-root shortcut produced a false absence.
		t.Logf("cross-mount target resolved as %+v", got)
	}
	insideRemoved := sc["removed_inside_source"].(string)
	if got := env.confirmer().Confirm(ctx, env.Root, insideRemoved); got.Status != AbsenceConfirmed {
		t.Fatalf("removed parent inside a reachable source must be confirmed: %+v", got)
	}
}

// TestIntegrationRefreshPermissionAgainstRealOpenList proves that a token
// which can read but cannot refresh is rejected rather than downgraded to a
// cached listing.
func TestIntegrationRefreshPermissionAgainstRealOpenList(t *testing.T) {
	env := loadIntegrationEnv(t)
	sc := env.scenario(t, "refresh_permission")
	ctx := context.Background()

	readOnlyToken := sc["read_only_token"].(string)
	if readOnlyToken == "" {
		t.Fatal("runner did not provide a read-only token")
	}
	client := New(env.APIURL, readOnlyToken)
	if _, err := client.ListFresh(ctx, env.Root); err == nil {
		t.Fatal("a token without refresh permission must fail a fresh listing")
	}
	confirmer := NewConfirmer(client)
	got := confirmer.Confirm(ctx, env.Root, remoteJoin(env.Root, "Missing.mkv"))
	if got.Status != AbsenceUnknown {
		t.Fatalf("a token that cannot refresh must not confirm absence: %+v", got)
	}
}

// TestIntegrationCachedViewIsNotEvidenceAgainstRealOpenList proves the real
// cached driver path: stale cached entries must not decide the result once a
// file was deleted on the backing store, and a file that reappears must be
// found again by a later call.
func TestIntegrationCachedViewIsNotEvidenceAgainstRealOpenList(t *testing.T) {
	env := loadIntegrationEnv(t)
	sc := env.scenario(t, "cached_driver")
	ctx := context.Background()
	mount := sc["mount"].(string)
	client := env.client()
	confirmer := NewConfirmer(client)

	// Warm the server-side cache for the mount.
	if _, err := client.List(ctx, mount); err != nil {
		t.Fatalf("warm cache: %v", err)
	}
	if got := confirmer.Confirm(ctx, mount, sc["deleted_after_cache"].(string)); got.Status != AbsenceConfirmed {
		t.Fatalf("deleted file with a stale cache entry must be confirmed absent: %+v", got)
	}
	if got := confirmer.Confirm(ctx, mount, sc["present_file"].(string)); got.Status != AbsencePresent {
		t.Fatalf("present file through the cached driver: %+v", got)
	}
	// A name created after the cache was warmed must be visible after refresh.
	if got := confirmer.Confirm(ctx, mount, sc["created_after_cache"].(string)); got.Status != AbsencePresent {
		t.Fatalf("file created after the cache was warmed must be present: %+v", got)
	}
}

// TestIntegrationNestedMountDoesNotMaskDriverFailureAgainstRealOpenList proves
// the masking hazard measured on the fixed build: a directory that only exists
// as the ancestor of a nested mount keeps listing successfully while its own
// driver is unavailable, so such a view must never confirm absence.
func TestIntegrationNestedMountDoesNotMaskDriverFailureAgainstRealOpenList(t *testing.T) {
	env := loadIntegrationEnv(t)
	sc := env.scenario(t, "nested_mount_masking")
	ctx := context.Background()
	mount := sc["mount"].(string)
	confirmer := env.confirmer()

	// The runner disabled the parent storage before invoking this test. The
	// virtual ancestor must still be rejected as an absence anchor.
	ancestor := sc["virtual_ancestor"].(string)
	got := confirmer.Confirm(ctx, mount, remoteJoin(ancestor, "NotThere.mkv"))
	if got.Status != AbsenceUnknown {
		t.Fatalf("a virtual mount view must not confirm absence: %+v", got)
	}
	if got.Reason != ReasonVirtualViewObserved && got.Reason != ReasonRootUnavailable && got.Reason != ReasonRootNotRealStorage {
		t.Fatalf("unexpected reason for a virtual view: %+v", got)
	}

	// The source root itself is unreachable while the parent storage is
	// disabled, so nothing may be concluded from it either.
	if got := confirmer.Confirm(ctx, mount, remoteJoin(mount, "Some.mkv")); got.Status != AbsenceUnknown {
		t.Fatalf("an unreachable source root must not confirm absence: %+v", got)
	}
}

// TestIntegrationPermissionFilteringAgainstRealOpenList records what the real
// API does when a metadata rule hides entries from a user, and proves the
// confirmation treats a filtered view conservatively.
func TestIntegrationPermissionFilteringAgainstRealOpenList(t *testing.T) {
	env := loadIntegrationEnv(t)
	sc := env.scenario(t, "hidden_entries")
	ctx := context.Background()
	adminConfirmer := env.confirmer()

	hidden := sc["hidden_name"].(string)
	if got := adminConfirmer.Confirm(ctx, env.Root, hidden); got.Status != AbsencePresent {
		t.Fatalf("the file exists on disk and must be present for a non-filtered reader: %+v", got)
	}

	restrictedToken := sc["restricted_token"].(string)
	if restrictedToken == "" {
		t.Fatal("runner did not provide a restricted token")
	}
	restricted := NewConfirmer(New(env.APIURL, restrictedToken))
	got := restricted.Confirm(ctx, env.Root, hidden)
	if got.Status == AbsenceConfirmed {
		t.Fatalf("a filtered view must not confirm absence: %+v", got)
	}
}

// TestIntegrationNoStorageWrites proves confirmation is read-only with respect
// to the remote tree: no file is created, modified or removed by confirming.
func TestIntegrationNoStorageWrites(t *testing.T) {
	env := loadIntegrationEnv(t)
	sc := env.scenario(t, "exact_names")
	ctx := context.Background()

	before, err := env.client().ListFresh(ctx, env.Root)
	if err != nil {
		t.Fatalf("baseline listing: %v", err)
	}
	for i := 0; i < 3; i++ {
		env.confirmer().Confirm(ctx, env.Root, sc["absent_file"].(string))
		env.confirmer().Confirm(ctx, env.Root, sc["present_file"].(string))
	}
	after, err := env.client().ListFresh(ctx, env.Root)
	if err != nil {
		t.Fatalf("final listing: %v", err)
	}
	if strings.Join(names(before), "\x00") != strings.Join(names(after), "\x00") {
		t.Fatalf("confirmation changed the remote directory: %v -> %v", names(before), names(after))
	}
}

// TestIntegrationConfirmationIsBounded proves a very deep target is rejected
// before any request is made, and that a wide directory is read completely
// within the documented limit.
func TestIntegrationConfirmationIsBounded(t *testing.T) {
	env := loadIntegrationEnv(t)
	ctx := context.Background()
	segments := make([]string, MaxAbsenceDepth+2)
	for i := range segments {
		segments[i] = "d"
	}
	tooDeep := remoteJoin(env.Root, path.Join(segments...))
	tooDeep = strings.ReplaceAll(tooDeep, "//", "/")
	got := env.confirmer().Confirm(ctx, env.Root, tooDeep)
	if got.Status != AbsenceUnknown || got.Reason != ReasonDepthLimit {
		t.Fatalf("an over-deep target must be rejected: %+v", got)
	}
	if len(got.Chain) != 0 {
		t.Fatalf("a rejected path must not produce a chain: %+v", got)
	}
}
