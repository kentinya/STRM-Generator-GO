package pathmap

import "testing"

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
