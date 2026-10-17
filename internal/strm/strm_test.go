package strm

import (
	"strings"
	"testing"
)

func TestParseRoundTrip(t *testing.T) {
	doc := &Document{URL: "https://example.invalid/d/A.mkv", SHA1: strings.Repeat("a", 40), HasSHA: true, Size: 4096, HasSize: true}
	raw := doc.Bytes()
	if !strings.HasSuffix(string(raw), "\n") {
		t.Fatalf("document must end with LF: %q", raw)
	}
	parsed, err := Parse(raw)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if parsed.URL != doc.URL || parsed.SHA1 != doc.SHA1 || parsed.Size != doc.Size || !parsed.HasSHA || !parsed.HasSize {
		t.Fatalf("round trip mismatch: %+v", parsed)
	}
	if strings.Contains(string(raw), "Modified") {
		t.Fatal("must not write Modified")
	}
}

func TestParseOptionalFieldsOmitted(t *testing.T) {
	doc := &Document{URL: "https://example.invalid/d/A.mkv"}
	raw := string(doc.Bytes())
	if strings.Contains(raw, "sha1") || strings.Contains(raw, "size") {
		t.Fatalf("optional fields must be omitted: %q", raw)
	}
	if !strings.HasPrefix(raw, "# strm-probe-version:1\n") {
		t.Fatalf("version must be first: %q", raw)
	}
}

func TestParseRejectsInvalid(t *testing.T) {
	cases := map[string]string{
		"missing version":   "https://example.invalid/d/A.mkv\n",
		"duplicate version": "# strm-probe-version:1\n# strm-probe-version:1\nhttps://example.invalid/d/A.mkv\n",
		"bad version":       "# strm-probe-version:2\nhttps://example.invalid/d/A.mkv\n",
		"bad sha":           "# strm-probe-version:1\n# strm-probe-sha1:xyz\nhttps://example.invalid/d/A.mkv\n",
		"uppercase sha":     "# strm-probe-version:1\n# strm-probe-sha1:" + strings.Repeat("A", 40) + "\nhttps://example.invalid/d/A.mkv\n",
		"negative size":     "# strm-probe-version:1\n# strm-probe-size:-1\nhttps://example.invalid/d/A.mkv\n",
		"multiple urls":     "# strm-probe-version:1\nhttps://example.invalid/d/A.mkv\nhttps://example.invalid/d/B.mkv\n",
		"non-http url":      "# strm-probe-version:1\nftp://example.invalid/A.mkv\n",
		"url with query":    "# strm-probe-version:1\nhttps://example.invalid/d/A.mkv?sign=x\n",
	}
	for name, raw := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := Parse([]byte(raw)); err == nil {
				t.Fatalf("expected error for %q", raw)
			}
		})
	}
}

func TestParseToleratesBOMAndCRLF(t *testing.T) {
	raw := "\uFEFF# strm-probe-version:1\r\n# strm-probe-size:0\r\nhttps://example.invalid/d/A.mkv\r\n"
	doc, err := Parse([]byte(raw))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !doc.HasSize || doc.Size != 0 {
		t.Fatalf("size zero must be retained: %+v", doc)
	}
}

func TestCompareMatrix(t *testing.T) {
	base := &Document{URL: "https://example.invalid/d/A.mkv", SHA1: strings.Repeat("a", 40), HasSHA: true, Size: 10, HasSize: true}
	cases := []struct {
		name     string
		existing *Document
		desired  *Document
		want     Decision
		anomaly  bool
	}{
		{"new file", nil, base, Update, false},
		{"identical", base, base, Unchanged, false},
		{"url change", base, &Document{URL: "https://example.invalid/d/B.mkv", SHA1: strings.Repeat("a", 40), HasSHA: true, Size: 10, HasSize: true}, Update, false},
		{"same sha size diff", base, &Document{URL: base.URL, SHA1: base.SHA1, HasSHA: true, Size: 99, HasSize: true}, Unchanged, true},
		{"sha differs", base, &Document{URL: base.URL, SHA1: strings.Repeat("b", 40), HasSHA: true, Size: 10, HasSize: true}, Update, false},
		{"sha incomparable size same", &Document{URL: base.URL, Size: 10, HasSize: true}, &Document{URL: base.URL, Size: 10, HasSize: true}, Unchanged, false},
		{"sha incomparable size differs", &Document{URL: base.URL, Size: 10, HasSize: true}, &Document{URL: base.URL, Size: 11, HasSize: true}, Update, false},
		{"insufficient", &Document{URL: base.URL}, &Document{URL: base.URL}, Unchanged, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Compare(tc.existing, tc.desired)
			if err != nil {
				t.Fatalf("compare: %v", err)
			}
			if got.Decision != tc.want {
				t.Fatalf("decision = %v want %v", got.Decision, tc.want)
			}
			if tc.anomaly && got.Anomaly == "" {
				t.Fatal("expected anomaly note")
			}
			if !tc.anomaly && got.Anomaly != "" {
				t.Fatalf("unexpected anomaly: %q", got.Anomaly)
			}
		})
	}
}

func TestValidateSizeBoundary(t *testing.T) {
	doc := &Document{URL: "https://example.invalid/d/A.mkv", Size: 9223372036854775807, HasSize: true}
	if err := doc.Validate(); err != nil {
		t.Fatalf("max int64 size must be valid: %v", err)
	}
}

func TestMaxSize(t *testing.T) {
	long := strings.Repeat("x", MaxSize)
	doc := &Document{URL: "https://example.invalid/d/" + long}
	if err := doc.Validate(); err == nil {
		t.Fatal("oversized document must be rejected")
	}
}
