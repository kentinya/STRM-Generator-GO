package pathmap

import (
	"strings"
	"testing"
)

func TestIsMedia(t *testing.T) {
	for _, name := range []string{"A.mkv", "A.MP4", "b.M4v", "c.m2ts"} {
		if !IsMedia(name) {
			t.Fatalf("%s should be media", name)
		}
	}
	for _, name := range []string{"notes.txt", "A.strm", "noext", "A.mkv.bak"} {
		if IsMedia(name) {
			t.Fatalf("%s should not be media", name)
		}
	}
}

func TestLocalRelative(t *testing.T) {
	cases := []struct {
		dir, name, want string
	}{
		{"", "A.mkv", "A.strm"},
		{"科幻", "A #100%.mkv", "科幻/A #100%.strm"},
		{"sub/deep", "B.mp4", "sub/deep/B.strm"},
	}
	for _, tc := range cases {
		got, err := LocalRelative(tc.dir, tc.name)
		if err != nil {
			t.Fatalf("LocalRelative(%q,%q): %v", tc.dir, tc.name, err)
		}
		if got != tc.want {
			t.Fatalf("LocalRelative(%q,%q) = %q want %q", tc.dir, tc.name, got, tc.want)
		}
	}
}

func TestLocalRelativeRejectsBadNames(t *testing.T) {
	for _, name := range []string{"", ".", "..", "a/b.mkv", "a\\b.mkv", "a\x00b.mkv"} {
		if _, err := LocalRelative("", name); err == nil {
			t.Fatalf("expected rejection for %q", name)
		}
	}
}

func TestLocalRelativeRejectsInvalidUTF8AndOversizedSegments(t *testing.T) {
	invalid := string([]byte{'A', '.', 'm', 'k', 'v', 0xff})
	if _, err := LocalRelative("", invalid); err == nil {
		t.Fatal("invalid UTF-8 name must be rejected")
	}
	tooLong := strings.Repeat("a", MaxSegmentBytes+1) + ".mkv"
	if _, err := LocalRelative("", tooLong); err == nil {
		t.Fatal("oversized path segment must be rejected")
	}
}

func TestStableURLEncodesSegments(t *testing.T) {
	got, err := StableURL("https://example.invalid", "/Movies/科幻/A #100%.mkv")
	if err != nil {
		t.Fatalf("StableURL: %v", err)
	}
	want := "https://example.invalid/d/Movies/%E7%A7%91%E5%B9%BB/A%20%23100%25.mkv"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestStableURLKeepsPrefix(t *testing.T) {
	got, err := StableURL("https://example.invalid/openlist", "/Movies/A.mkv")
	if err != nil {
		t.Fatalf("StableURL: %v", err)
	}
	if got != "https://example.invalid/openlist/d/Movies/A.mkv" {
		t.Fatalf("prefix lost: %q", got)
	}
}

func TestStableURLRejectsRelative(t *testing.T) {
	if _, err := StableURL("https://example.invalid", "Movies/A.mkv"); err == nil {
		t.Fatal("relative remote path must be rejected")
	}
	if _, err := StableURL("https://example.invalid", "/Movies/../etc/passwd"); err == nil {
		t.Fatal("relative segment must be rejected")
	}
}

func TestDefaultExtensionsCopyIsIsolated(t *testing.T) {
	first := DefaultExtensions()
	if len(first) != 11 {
		t.Fatalf("default list = %v", first)
	}
	first[0] = ".zzz"
	second := DefaultExtensions()
	if second[0] != ".mkv" {
		t.Fatalf("default list was mutated: %v", second)
	}
	if len(MediaExtensions) != 11 {
		t.Fatalf("shared default map changed: %v", MediaExtensions)
	}
}

func TestMediaSetMatchesOnlyLastExtension(t *testing.T) {
	set := NewMediaSet([]string{".flv", ".mkv"})
	for _, name := range []string{"A.flv", "B.FLV", "C.Mkv", "dir.flv/..", "D.mkv"} {
		if name == "dir.flv/.." {
			continue
		}
		if !set.IsMedia(name) {
			t.Fatalf("%s should match the instance set", name)
		}
	}
	for _, name := range []string{"A.mp4", "A.mkv.bak", "A.flv.tmp", "noext", "A.strm", "A.flv "} {
		if set.IsMedia(name) {
			t.Fatalf("%s should not match the instance set", name)
		}
	}
	// The default set is untouched by instance construction.
	if !IsMedia("A.mp4") {
		t.Fatal("instance set changed the default matching")
	}
}

func TestMediaSetZeroValueFallsBackToDefault(t *testing.T) {
	var nilSet *MediaSet
	if !nilSet.IsMedia("A.mkv") {
		t.Fatal("nil set must fall back to the default list")
	}
	empty := NewMediaSet(nil)
	if !empty.IsMedia("A.mp4") || empty.IsMedia("A.flv") {
		t.Fatal("empty set must fall back to the default list")
	}
}

func TestMediaSetsDoNotPolluteEachOther(t *testing.T) {
	first := NewMediaSet([]string{".flv"})
	second := NewMediaSet([]string{".mp3"})
	if !first.IsMedia("A.flv") || first.IsMedia("A.mp3") {
		t.Fatal("first instance set is wrong")
	}
	if !second.IsMedia("A.mp3") || second.IsMedia("A.flv") {
		t.Fatal("second instance set is wrong")
	}
	if first.IsMedia("A.mkv") || second.IsMedia("A.mkv") {
		t.Fatal("an explicit instance set fell back to defaults")
	}
}
