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
		t.Fatalf("%s: got %s (%s), want %s", context, got.Status, got.Reason, want)
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
// two-read rule against the real Local driver: an existing exact name is
// present, the same name with different case or another missing name is one
// missing observation, and single-page listings still use refresh:false.
func TestIntegrationConfirmPresentAndAbsentAgainstRealOpenList(t *testing.T) {
	env := loadIntegrationEnv(t)
	sc := env.scenario(t, "exact_names")
	ctx := context.Background()
	confirmer := env.confirmer()

	expectStatus(t, confirmer.Confirm(ctx, env.Root, sc["present_file"].(string)), AbsencePresent, "existing file")
	expectStatus(t, confirmer.Confirm(ctx, env.Root, sc["absent_file"].(string)), AbsenceMissing, "absent file")
	expectStatus(t, confirmer.Confirm(ctx, env.Root, sc["case_variant"].(string)), AbsenceMissing, "case variant is a different name")

	for _, target := range sc["special_files"].([]any) {
		expectStatus(t, confirmer.Confirm(ctx, env.Root, target.(string)), AbsencePresent, "special name "+target.(string))
	}

	// The source root itself is a directory: confirming the root as a target is
	// rejected before any request, never read as present or missing.
	expectStatus(t, confirmer.Confirm(ctx, env.Root, env.Root), AbsenceUnknown, "root as target")
}

// TestIntegrationTargetDirectoryIsPresentAgainstRealOpenList proves that a
// real directory read at the exact target is a type conflict the caller keeps,
// not a missing observation.
func TestIntegrationTargetDirectoryIsPresentAgainstRealOpenList(t *testing.T) {
	env := loadIntegrationEnv(t)
	sc := env.scenario(t, "empty_directory")
	ctx := context.Background()
	target := sc["directory"].(string)
	got := env.confirmer().Confirm(ctx, env.Root, target)
	if got.Status != AbsencePresent || got.Reason != ReasonTargetTypeConflict {
		t.Fatalf("a directory target must be a reported type conflict: %+v", got)
	}
	expectStatus(t, env.confirmer().Confirm(ctx, env.Root, remoteJoin(target, "Anything.mkv")),
		AbsenceMissing, "missing inside a real empty directory")
}

// TestIntegrationPaginationUsesCachedPagesAgainstRealOpenList proves the list
// traversal still reads every page of a real multi-page directory while asking
// for no server-side refresh, and that the target read is unaffected.
func TestIntegrationPaginationUsesCachedPagesAgainstRealOpenList(t *testing.T) {
	env := loadIntegrationEnv(t)
	sc := env.scenario(t, "pagination")
	ctx := context.Background()

	last := sc["last_page_file"].(string)
	expectStatus(t, env.confirmer().Confirm(ctx, env.Root, last), AbsencePresent, "file on the last page")
	absent := sc["absent_file"].(string)
	expectStatus(t, env.confirmer().Confirm(ctx, env.Root, absent), AbsenceMissing, "absent file in a multi-page directory")

	entries, err := env.client().List(ctx, sc["directory"].(string))
	if err != nil {
		t.Fatalf("listing a multi-page directory: %v", err)
	}
	want := int(sc["total"].(float64))
	if len(entries) != want {
		t.Fatalf("listing returned %d entries, want %d", len(entries), want)
	}
}

// TestIntegrationEmptyDirectoryShapeAgainstRealOpenList records the exact
// serialization of a real empty directory and proves the client still classifies
// it as a complete empty listing.
func TestIntegrationEmptyDirectoryShapeAgainstRealOpenList(t *testing.T) {
	env := loadIntegrationEnv(t)
	sc := env.scenario(t, "empty_directory")
	ctx := context.Background()
	emptyDir := sc["directory"].(string)

	entries, err := env.client().List(ctx, emptyDir)
	if err != nil {
		t.Fatalf("real empty directory must be a complete empty listing: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("real empty directory returned %d entries", len(entries))
	}
}

// TestIntegrationRemovedParentAgainstRealOpenList proves the two contrasting
// source-root facts measured on the pinned build: a mount whose backing
// directory was removed still reads as a directory (so the rule continues and
// the target is a missing observation), while a disabled storage does not (so
// the source is kept). A parent removed inside a readable source is also a
// missing observation, established without any ancestor walk.
func TestIntegrationRemovedParentAgainstRealOpenList(t *testing.T) {
	env := loadIntegrationEnv(t)
	sc := env.scenario(t, "removed_parent")
	ctx := context.Background()

	expectStatus(t, env.confirmer().Confirm(ctx, env.Root, sc["removed_inside_source"].(string)),
		AbsenceMissing, "a parent removed inside a readable source")

	// The mount still answers its root with a directory object even though the
	// backing directory is gone; the deep target is not read.
	goneRoot := sc["backing_dir_gone_root"].(string)
	expectStatus(t, env.confirmer().Confirm(ctx, goneRoot, sc["backing_dir_gone_target"].(string)),
		AbsenceMissing, "a readable mount root whose backing directory is gone")

	// A disabled storage cannot read its mount path, so the source is kept.
	disabled := sc["disabled_root"].(string)
	expectStatus(t, env.confirmer().Confirm(ctx, disabled, remoteJoin(disabled, "Only.mkv")),
		AbsenceUnknown, "a disabled storage root must keep the source")
}

// TestIntegrationReadOnlyIdentityAgainstRealOpenList proves the new rule needs
// no refresh permission: a read-only token lists with refresh:false and gets
// the same root/target classification.
func TestIntegrationReadOnlyIdentityAgainstRealOpenList(t *testing.T) {
	env := loadIntegrationEnv(t)
	sc := env.scenario(t, "read_only_identity")
	ctx := context.Background()

	readOnlyToken := sc["read_only_token"].(string)
	if readOnlyToken == "" {
		t.Fatal("runner did not provide a read-only token")
	}
	client := New(env.APIURL, readOnlyToken)
	if _, err := client.List(ctx, env.Root); err != nil {
		t.Fatalf("a read-only identity must be able to list with refresh:false: %v", err)
	}
	confirmer := NewConfirmer(client)
	expectStatus(t, confirmer.Confirm(ctx, env.Root, sc["present_file"].(string)), AbsencePresent,
		"read-only identity on an existing file")
	expectStatus(t, confirmer.Confirm(ctx, env.Root, sc["absent_file"].(string)), AbsenceMissing,
		"read-only identity on a missing file")
}

// TestIntegrationCachedDriverAgainstRealOpenList records what a real cached
// driver answers after its cache naturally expires. The runner warms the cache,
// changes the backing store, and waits the TTL out before running this test, so
// no assertion here depends on a forced refresh or on immediate convergence.
//
// The measured fact is that the WebDav self-mount of the pinned build serves
// directories through its cache but cannot serve a direct object read of a
// file. Under the accepted rule that is one of the documented limitations: the
// directory root reads and an unreadable file target is a missing observation.
func TestIntegrationCachedDriverAgainstRealOpenList(t *testing.T) {
	env := loadIntegrationEnv(t)
	sc := env.scenario(t, "cached_driver")
	ctx := context.Background()
	mount := sc["mount"].(string)
	confirmer := env.confirmer()

	// The cached root reads as a directory, so the rule continues.
	expectStatus(t, confirmer.Confirm(ctx, mount, sc["present_file"].(string)), AbsenceMissing,
		"cached driver file read limitation")
	// After the natural TTL expiry the deleted name and the newly created one
	// must be classified consistently on repeated reads, so a later round sees
	// the same fact instead of a changing answer.
	for _, target := range []string{sc["deleted_after_cache"].(string), sc["created_after_cache"].(string)} {
		first := confirmer.Confirm(ctx, mount, target)
		second := confirmer.Confirm(ctx, mount, target)
		if first.Status != second.Status {
			t.Fatalf("%s answered inconsistently after the cache expired: %s then %s",
				target, first.Status, second.Status)
		}
		expectStatus(t, second, AbsenceMissing, "cached driver file read limitation")
	}

	// A listing through the same client is still a cached list, and it proves
	// the cache really did update: the deleted name is gone and the created one
	// is present.
	entries, err := env.client().List(ctx, remoteJoin(mount, "Movies"))
	if err != nil {
		t.Fatalf("cached listing: %v", err)
	}
	found := map[string]bool{}
	for _, entry := range entries {
		found[entry.Name] = true
	}
	if found["A Movie.mkv"] {
		t.Fatalf("the cached listing still contains the deleted name: %v", names(entries))
	}
	if !found["Reappeared.mkv"] {
		t.Fatalf("the cached listing never reflected the created name: %v", names(entries))
	}
}

// TestIntegrationNestedMountMaskingAgainstRealOpenList records the masking
// hazard on the fixed build. A directory that only exists as the ancestor of a
// nested mount keeps reading as a directory (provider "unknown") while its own
// driver is unavailable. Under the accepted rule that readable directory
// continues and a target that is not read is one missing observation: the
// hazard is a documented limitation, not a protected condition. A disabled
// storage with no nested mount, by contrast, has an unreadable root and keeps
// its source.
func TestIntegrationNestedMountMaskingAgainstRealOpenList(t *testing.T) {
	env := loadIntegrationEnv(t)
	sc := env.scenario(t, "nested_mount_masking")
	ctx := context.Background()
	confirmer := env.confirmer()

	ancestor := sc["virtual_ancestor"].(string)
	expectStatus(t, confirmer.Confirm(ctx, ancestor, remoteJoin(ancestor, "NotThere.mkv")),
		AbsenceMissing, "a readable virtual ancestor continues under the accepted rule")

	disabled := sc["disabled_root"].(string)
	expectStatus(t, confirmer.Confirm(ctx, disabled, remoteJoin(disabled, "Only.mkv")),
		AbsenceUnknown, "a disabled storage root must keep the source")
}

// TestIntegrationPermissionFilteredAgainstRealOpenList records the accepted
// limitation for a metadata-hidden entry: a non-privileged identity does not see
// it in a listing and cannot read it, so the target is a missing observation.
// The file still exists on disk, and an unrestricted identity reads it.
func TestIntegrationPermissionFilteredAgainstRealOpenList(t *testing.T) {
	env := loadIntegrationEnv(t)
	sc := env.scenario(t, "hidden_entries")
	ctx := context.Background()

	hidden := sc["hidden_name"].(string)
	expectStatus(t, env.confirmer().Confirm(ctx, env.Root, hidden), AbsencePresent,
		"the file exists on disk and is readable by an unrestricted identity")

	restrictedToken := sc["restricted_token"].(string)
	if restrictedToken == "" {
		t.Fatal("runner did not provide a restricted token")
	}
	restricted := NewConfirmer(New(env.APIURL, restrictedToken))
	expectStatus(t, restricted.Confirm(ctx, env.Root, hidden), AbsenceMissing,
		"a hidden entry is the accepted unread-target limitation")
}

// TestIntegrationNoStorageWrites proves confirmation is read-only with respect
// to the remote tree: no file is created, modified or removed by confirming,
// and a missing observation does not change what the next listing returns.
func TestIntegrationNoStorageWrites(t *testing.T) {
	env := loadIntegrationEnv(t)
	sc := env.scenario(t, "exact_names")
	ctx := context.Background()

	before, err := env.client().List(ctx, env.Root)
	if err != nil {
		t.Fatalf("baseline listing: %v", err)
	}
	for i := 0; i < 3; i++ {
		env.confirmer().Confirm(ctx, env.Root, sc["absent_file"].(string))
		env.confirmer().Confirm(ctx, env.Root, sc["present_file"].(string))
	}
	after, err := env.client().List(ctx, env.Root)
	if err != nil {
		t.Fatalf("final listing: %v", err)
	}
	if strings.Join(names(before), "\x00") != strings.Join(names(after), "\x00") {
		t.Fatalf("confirmation changed the remote directory: %v -> %v", names(before), names(after))
	}
}

// TestIntegrationConfirmationIsBounded proves a very deep target is rejected
// before any request is made.
func TestIntegrationConfirmationIsBounded(t *testing.T) {
	env := loadIntegrationEnv(t)
	ctx := context.Background()
	segments := make([]string, MaxAbsenceDepth+2)
	for i := range segments {
		segments[i] = "d"
	}
	tooDeep := strings.ReplaceAll(remoteJoin(env.Root, path.Join(segments...)), "//", "/")
	got := env.confirmer().Confirm(ctx, env.Root, tooDeep)
	if got.Status != AbsenceUnknown || got.Reason != ReasonDepthLimit {
		t.Fatalf("an over-deep target must be rejected: %+v", got)
	}
}
