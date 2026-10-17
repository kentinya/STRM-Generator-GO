// Package strm implements the version 1 STRM protocol: strict parsing,
// deterministic serialization and the comparison rules that decide whether an
// existing file is unchanged, must be updated, or cannot be judged.
package strm

import (
	"bytes"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// MaxSize is the plugin's 64 KiB parse limit; generated documents must not
// exceed it.
const MaxSize = 65536

const (
	versionPrefix = "# strm-probe-version:"
	sha1Prefix    = "# strm-probe-sha1:"
	sizePrefix    = "# strm-probe-size:"
)

// Document is a parsed or to-be-serialized STRM file.
type Document struct {
	URL     string
	SHA1    string // lowercase 40 hex chars, or "" when unavailable
	HasSHA  bool
	Size    int64
	HasSize bool
}

// ValidationError reports a document that cannot be trusted.
type ValidationError struct{ Reason string }

func (e *ValidationError) Error() string { return e.Reason }

// Validate checks every output invariant. It is used before writing a document
// and when deciding whether an existing file is parseable.
func (d *Document) Validate() error {
	if err := ValidateURL(d.URL); err != nil {
		return err
	}
	if d.HasSHA {
		if err := ValidateSHA1(d.SHA1); err != nil {
			return err
		}
	} else if d.SHA1 != "" {
		return &ValidationError{"sha1 present without flag"}
	}
	if d.HasSize {
		if d.Size < 0 {
			return &ValidationError{"size must not be negative"}
		}
	} else if d.Size != 0 {
		return &ValidationError{"size present without flag"}
	}
	if len(d.Bytes()) > MaxSize {
		return &ValidationError{"document exceeds 65536 bytes"}
	}
	return nil
}

// ValidateURL accepts only a clean absolute http(s) URL.
func ValidateURL(raw string) error {
	if raw == "" {
		return &ValidationError{"url is empty"}
	}
	if strings.ContainsAny(raw, "\r\n") {
		return &ValidationError{"url contains a newline"}
	}
	u, err := url.Parse(raw)
	if err != nil {
		return &ValidationError{"url is not a valid URL"}
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return &ValidationError{"url must use http or https"}
	}
	if u.Host == "" {
		return &ValidationError{"url must include a host"}
	}
	if u.User != nil {
		return &ValidationError{"url must not contain user information"}
	}
	if u.RawQuery != "" || u.ForceQuery {
		return &ValidationError{"url must not contain a query string"}
	}
	if u.Fragment != "" {
		return &ValidationError{"url must not contain a fragment"}
	}
	return nil
}

// ValidateSHA1 requires exactly 40 lowercase hex characters.
func ValidateSHA1(value string) error {
	if len(value) != 40 {
		return &ValidationError{"sha1 must be 40 hex characters"}
	}
	if value != strings.ToLower(value) {
		return &ValidationError{"sha1 must be lowercase"}
	}
	if _, err := hex.DecodeString(value); err != nil {
		return &ValidationError{"sha1 must be hexadecimal"}
	}
	return nil
}

// Bytes renders the canonical UTF-8/LF document. Fields are emitted in the
// fixed order version, sha1, size, URL; absent optional fields are omitted.
func (d *Document) Bytes() []byte {
	var b bytes.Buffer
	b.WriteString(versionPrefix)
	b.WriteString("1\n")
	if d.HasSHA {
		b.WriteString(sha1Prefix)
		b.WriteString(d.SHA1)
		b.WriteByte('\n')
	}
	if d.HasSize {
		b.WriteString(sizePrefix)
		b.WriteString(strconv.FormatInt(d.Size, 10))
		b.WriteByte('\n')
	}
	b.WriteString(d.URL)
	b.WriteByte('\n')
	return b.Bytes()
}

// Parse reads a document. It tolerates a UTF-8 BOM and CRLF line endings (the
// plugin reader does) but rejects duplicate or malformed markers and any
// non-optional invalid value.
func Parse(raw []byte) (*Document, error) {
	raw = bytes.TrimPrefix(raw, []byte{0xEF, 0xBB, 0xBF})
	text := strings.ReplaceAll(string(raw), "\r\n", "\n")
	text = strings.TrimRight(text, "\n")
	if text == "" {
		return nil, &ValidationError{"document is empty"}
	}
	lines := strings.Split(text, "\n")
	doc := &Document{}
	seenVersion, seenSHA, seenSize := false, false, false
	var urlLine string
	for _, line := range lines {
		switch {
		case strings.HasPrefix(line, versionPrefix):
			if seenVersion {
				return nil, &ValidationError{"duplicate version marker"}
			}
			seenVersion = true
			if strings.TrimPrefix(line, versionPrefix) != "1" {
				return nil, &ValidationError{"unsupported protocol version"}
			}
		case strings.HasPrefix(line, sha1Prefix):
			if seenSHA {
				return nil, &ValidationError{"duplicate sha1 marker"}
			}
			seenSHA = true
			value := strings.TrimPrefix(line, sha1Prefix)
			if err := ValidateSHA1(value); err != nil {
				return nil, err
			}
			doc.SHA1 = value
			doc.HasSHA = true
		case strings.HasPrefix(line, sizePrefix):
			if seenSize {
				return nil, &ValidationError{"duplicate size marker"}
			}
			seenSize = true
			value := strings.TrimPrefix(line, sizePrefix)
			n, err := strconv.ParseInt(value, 10, 64)
			if err != nil || n < 0 {
				return nil, &ValidationError{"size is not a non-negative Int64"}
			}
			doc.Size = n
			doc.HasSize = true
		case strings.HasPrefix(line, "#"):
			return nil, &ValidationError{"unexpected comment line"}
		default:
			if urlLine != "" {
				return nil, &ValidationError{"multiple URL lines"}
			}
			urlLine = line
		}
	}
	if !seenVersion {
		return nil, &ValidationError{"missing version marker"}
	}
	if urlLine == "" {
		return nil, &ValidationError{"missing URL line"}
	}
	doc.URL = urlLine
	if err := ValidateURL(doc.URL); err != nil {
		return nil, err
	}
	return doc, nil
}

// Decision is the outcome of comparing an existing document with a desired
// one, following the stable requirement priority URL -> SHA-1 -> Size.
type Decision int

const (
	// Unchanged means the existing bytes must not be rewritten.
	Unchanged Decision = iota
	// Update means the file content must change.
	Update
	// Conflict means the existing file is not a trustworthy owned STRM.
	Conflict
	// NeedsUpdate means an update is required but this task does not perform
	// content replacement; the caller must fail without touching the file.
	NeedsUpdate
)

// CompareResult carries the decision plus an optional anomaly note.
type CompareResult struct {
	Decision Decision
	Anomaly  string // non-empty for the same-hash/different-size case
}

// Compare applies the stable comparison rules. existing may be nil when no
// file is present. It returns an error when existing is present but cannot be
// parsed.
func Compare(existing *Document, desired *Document) (CompareResult, error) {
	if existing == nil {
		return CompareResult{Decision: Update}, nil
	}
	if err := existing.Validate(); err != nil {
		return CompareResult{Decision: Conflict}, err
	}
	if existing.URL != desired.URL {
		return CompareResult{Decision: Update}, nil
	}
	// URL is identical. Compare SHA-1 when both sides have a valid hash.
	if existing.HasSHA && desired.HasSHA {
		if existing.SHA1 == desired.SHA1 {
			if existing.HasSize && desired.HasSize && existing.Size != desired.Size {
				return CompareResult{
					Decision: Unchanged,
					Anomaly:  fmt.Sprintf("same sha1 but size differs (existing=%d desired=%d)", existing.Size, desired.Size),
				}, nil
			}
			return CompareResult{Decision: Unchanged}, nil
		}
		return CompareResult{Decision: Update}, nil
	}
	// SHA-1 is not comparable. Fall back to a valid Size comparison.
	if existing.HasSize && desired.HasSize {
		if existing.Size == desired.Size {
			return CompareResult{Decision: Unchanged}, nil
		}
		return CompareResult{Decision: Update}, nil
	}
	// Not enough information to justify a change.
	return CompareResult{Decision: Unchanged, Anomaly: "insufficient metadata to compare; keeping existing file"}, nil
}

// ErrNotExist is a sentinel used by callers when a target file is absent.
var ErrNotExist = errors.New("target does not exist")
