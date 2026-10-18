// Package openlist implements the OpenList list/get API client with bounded
// HTTP usage, explicit pagination and careful metadata handling. It never
// copies raw_url or signature parameters and never downloads media.
package openlist

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	// DefaultTimeout bounds a single HTTP request.
	DefaultTimeout = 30 * time.Second
	// MaxResponseBytes bounds one API response body.
	MaxResponseBytes = 8 << 20
	// perPage is the explicit page size used for directory listing.
	perPage = 200
	// MaxDirectoryEntries bounds the accumulated objects for one directory.
	// At most 10,000 entries are retained by one List call.
	MaxDirectoryEntries = 10000
	// MaxDirectoryNameBytes bounds names retained for duplicate/conflict checks.
	MaxDirectoryNameBytes = 2 << 20
	// maxPages bounds a single directory traversal to avoid unbounded loops.
	maxPages = MaxDirectoryEntries/perPage + 1
)

// APIError reports a transport, HTTP or application-level API failure.
type APIError struct {
	Op     string
	Detail string
	// Code is the OpenList application code from the response envelope, or 0
	// when the failure happened before an envelope was decoded. It is a
	// structural field: callers must branch on it rather than on message text,
	// which is free-form and may describe any driver failure.
	Code int
	// HTTPStatus is the transport status, 0 when no response was received.
	HTTPStatus int
}

func (e *APIError) Error() string { return e.Op + ": " + e.Detail }

// Entry is one directory entry with optional metadata.
type Entry struct {
	Name    string
	IsDir   bool
	Size    *int64
	SHA1    string // lowercase 40 hex chars, empty when unavailable/invalid
	HasSHA  bool
	Anomaly string // limited reason when optional metadata is unusable
}

// Client is a bounded OpenList API client.
type Client struct {
	apiURL   string
	token    string
	http     *http.Client
	maxBytes int64
}

// New builds a client. apiURL is the validated API base (no query/fragment).
func New(apiURL, token string) *Client {
	return &Client{
		apiURL:   strings.TrimRight(apiURL, "/"),
		token:    token,
		http:     &http.Client{Timeout: DefaultTimeout},
		maxBytes: MaxResponseBytes,
	}
}

// WithHTTPClient replaces the underlying HTTP client (used by tests).
func (c *Client) WithHTTPClient(hc *http.Client) *Client {
	c.http = hc
	return c
}

type apiEnvelope struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

// do performs one POST and returns the decoded data payload. It checks the
// transport status and the outer JSON code, and rejects null/oversized data.
func (c *Client) do(ctx context.Context, endpoint string, request any) (json.RawMessage, error) {
	body, err := json.Marshal(request)
	if err != nil {
		return nil, &APIError{Op: endpoint, Detail: "encode request"}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.apiURL+endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, &APIError{Op: endpoint, Detail: "build request"}
	}
	req.Header.Set("Authorization", c.token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, &APIError{Op: endpoint, Detail: "transport failure"}
	}
	defer resp.Body.Close()
	limited := io.LimitReader(resp.Body, c.maxBytes+1)
	raw, err := io.ReadAll(limited)
	if err != nil {
		return nil, &APIError{Op: endpoint, Detail: "read response"}
	}
	if int64(len(raw)) > c.maxBytes {
		return nil, &APIError{Op: endpoint, Detail: "response exceeds byte limit"}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, &APIError{Op: endpoint, Detail: fmt.Sprintf("http status %d", resp.StatusCode), HTTPStatus: resp.StatusCode}
	}
	var env apiEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, &APIError{Op: endpoint, Detail: "invalid JSON envelope", HTTPStatus: resp.StatusCode}
	}
	if env.Code != 200 {
		return nil, &APIError{Op: endpoint, Detail: fmt.Sprintf("application code %d", env.Code), Code: env.Code, HTTPStatus: resp.StatusCode}
	}
	if len(env.Data) == 0 || string(env.Data) == "null" {
		return nil, &APIError{Op: endpoint, Detail: "missing data payload"}
	}
	return env.Data, nil
}

// Get reads metadata for a single remote path.
func (c *Client) Get(ctx context.Context, remotePath string) (*Entry, error) {
	data, err := c.do(ctx, "/api/fs/get", map[string]any{"path": remotePath})
	if err != nil {
		return nil, err
	}
	var raw rawEntry
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, &APIError{Op: "/api/fs/get", Detail: "invalid object shape"}
	}
	return raw.toEntry()
}

type rawEntry struct {
	Name     string          `json:"name"`
	IsDir    *bool           `json:"is_dir"`
	Size     json.RawMessage `json:"size"`
	HashInfo json.RawMessage `json:"hash_info"`
	Hashinfo json.RawMessage `json:"hashinfo"`
}

// toEntry converts one raw object.
func (r *rawEntry) toEntry() (*Entry, error) {
	if r.Name == "" {
		return nil, &APIError{Op: "entry", Detail: "missing name"}
	}
	if r.IsDir == nil {
		return nil, &APIError{Op: "entry", Detail: "missing is_dir"}
	}
	size, sizeAnomaly := decodeSize(r.Size)
	e := &Entry{Name: r.Name, IsDir: *r.IsDir, Size: size}
	sha, ok, anomaly := decodeSHA1(r.HashInfo, r.Hashinfo)
	e.SHA1, e.HasSHA = sha, ok
	e.Anomaly = joinAnomalies(anomaly, sizeAnomaly)
	return e, nil
}

func decodeSize(raw json.RawMessage) (*int64, string) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 {
		return nil, "size is missing"
	}
	if bytes.Equal(raw, []byte("null")) {
		return nil, "size is unavailable"
	}
	n, err := strconv.ParseInt(string(raw), 10, 64)
	if err != nil || n < 0 {
		return nil, "size is not a non-negative Int64"
	}
	return &n, ""
}

func joinAnomalies(a, b string) string {
	if a == "" {
		return b
	}
	if b == "" {
		return a
	}
	return a + "; " + b
}

// decodeSHA1 prefers the structured hash_info.sha1 field and only falls back
// to the historical hashinfo JSON string. Invalid or missing values are
// omitted with a limited reason; zero values are never fabricated.
func decodeSHA1(hashInfo, hashinfo json.RawMessage) (string, bool, string) {
	if len(hashInfo) > 0 && string(hashInfo) != "null" {
		var obj map[string]any
		if err := json.Unmarshal(hashInfo, &obj); err == nil {
			if value, ok := obj["sha1"]; ok && value != nil {
				if s, ok := value.(string); ok {
					if normalized, ok := normalizeSHA1(s); ok {
						return normalized, true, ""
					}
					return "", false, "hash_info.sha1 is not a valid content hash"
				}
				return "", false, "hash_info.sha1 has unexpected type"
			}
			return "", false, "hash_info has no sha1"
		}
		return "", false, "hash_info is not a JSON object"
	}
	if len(hashinfo) > 0 && string(hashinfo) != "null" {
		var s string
		if err := json.Unmarshal(hashinfo, &s); err != nil {
			return "", false, "hashinfo is not a JSON string"
		}
		var obj map[string]any
		if err := json.Unmarshal([]byte(s), &obj); err != nil {
			return "", false, "hashinfo is not a JSON object"
		}
		if value, ok := obj["sha1"]; ok && value != nil {
			if str, ok := value.(string); ok {
				if normalized, ok := normalizeSHA1(str); ok {
					return normalized, true, ""
				}
			}
		}
		return "", false, "hashinfo has no valid sha1"
	}
	return "", false, ""
}

func normalizeSHA1(value string) (string, bool) {
	value = strings.TrimSpace(value)
	if len(value) != 40 {
		return "", false
	}
	if _, err := hex.DecodeString(value); err != nil {
		return "", false
	}
	return strings.ToLower(value), true
}

// List walks every page of a remote directory using the server-side cache.
// A directory is only reported complete when all pages succeed, the total
// stays consistent and the final page terminates exactly at the declared
// total.
//
// Every page is requested with an explicit refresh:false. This is the only
// listing mode the generator uses: no caller may force a server-side refresh,
// and there is no separate fresh-listing entry point to fall back to.
func (c *Client) List(ctx context.Context, remotePath string) ([]Entry, error) {
	var all []Entry
	seen := map[string]struct{}{}
	var expectedTotal *int64
	var nameBytes int64
	page := 1
	for {
		if page > maxPages {
			return nil, &APIError{Op: "/api/fs/list", Detail: "page limit exceeded"}
		}
		data, err := c.do(ctx, "/api/fs/list", map[string]any{
			"path": remotePath, "page": page, "per_page": perPage, "refresh": false,
		})
		if err != nil {
			return nil, err
		}
		var parsed struct {
			Content json.RawMessage `json:"content"`
			Total   json.RawMessage `json:"total"`
		}
		var total int64
		if err := json.Unmarshal(data, &parsed); err != nil {
			return nil, &APIError{Op: "/api/fs/list", Detail: "invalid list shape"}
		}
		if len(parsed.Content) == 0 {
			return nil, &APIError{Op: "/api/fs/list", Detail: "missing content"}
		}
		if len(parsed.Total) == 0 || bytes.Equal(bytes.TrimSpace(parsed.Total), []byte("null")) ||
			json.Unmarshal(parsed.Total, &total) != nil || total < 0 {
			return nil, &APIError{Op: "/api/fs/list", Detail: "missing or invalid total"}
		}
		// A real empty directory is serialized as content:null,total:0 by the
		// fixed OpenList build (verified against the pinned image version). That
		// exact shape is the only accepted null: null content with a non-zero
		// total, or null content on any later page, stays an error so a truncated
		// or partially paginated response can never look like an empty directory.
		var content []rawEntry
		if bytes.Equal(bytes.TrimSpace(parsed.Content), []byte("null")) {
			if total != 0 || page != 1 {
				return nil, &APIError{Op: "/api/fs/list", Detail: "null content for a non-empty directory"}
			}
		} else if err := json.Unmarshal(parsed.Content, &content); err != nil || content == nil {
			return nil, &APIError{Op: "/api/fs/list", Detail: "content is not an array"}
		}
		if expectedTotal == nil {
			expectedTotal = &total
			if total > MaxDirectoryEntries {
				return nil, &APIError{Op: "/api/fs/list", Detail: "directory entry limit exceeded"}
			}
		} else if total != *expectedTotal {
			return nil, &APIError{Op: "/api/fs/list", Detail: "total changed during traversal"}
		}
		if len(content) == 0 {
			if int64(len(all)) < *expectedTotal {
				return nil, &APIError{Op: "/api/fs/list", Detail: "page returned no content before total was reached"}
			}
			break
		}
		if len(content) > perPage {
			return nil, &APIError{Op: "/api/fs/list", Detail: "page exceeds per_page"}
		}
		for i := range content {
			entry, err := content[i].toEntry()
			if err != nil {
				return nil, err
			}
			if _, dup := seen[entry.Name]; dup {
				return nil, &APIError{Op: "/api/fs/list", Detail: "duplicate entry name"}
			}
			nameBytes += int64(len(entry.Name))
			if nameBytes > MaxDirectoryNameBytes {
				return nil, &APIError{Op: "/api/fs/list", Detail: "directory name byte limit exceeded"}
			}
			seen[entry.Name] = struct{}{}
			all = append(all, *entry)
		}
		if len(all) > MaxDirectoryEntries {
			return nil, &APIError{Op: "/api/fs/list", Detail: "directory entry limit exceeded"}
		}
		if int64(len(all)) > *expectedTotal {
			return nil, &APIError{Op: "/api/fs/list", Detail: "received more entries than total"}
		}
		if int64(len(all)) == *expectedTotal {
			break
		}
		page++
	}
	if expectedTotal == nil {
		return nil, &APIError{Op: "/api/fs/list", Detail: "no pages returned"}
	}
	return all, nil
}

// IsNotFound reports whether an error text mentions a remote 404.
//
// Deprecated: IsNotFound is not an absence signal and must never gate a
// deletion. The pinned OpenList build answers a missing object with HTTP 200
// and application code 500, and reports other failures the same way, so this
// text check cannot distinguish the cases. Use Confirmer, which classifies the
// configured source-root and target reads. This helper remains only for callers
// that merely need to shape a diagnostic.
func IsNotFound(err error) bool {
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		return strings.Contains(apiErr.Detail, "404")
	}
	return false
}
