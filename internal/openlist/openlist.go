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
	// maxPages bounds a single directory traversal to avoid unbounded loops.
	maxPages = 100000
)

// APIError reports a transport, HTTP or application-level API failure.
type APIError struct {
	Op     string
	Detail string
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
		return nil, &APIError{Op: endpoint, Detail: fmt.Sprintf("http status %d", resp.StatusCode)}
	}
	var env apiEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, &APIError{Op: endpoint, Detail: "invalid JSON envelope"}
	}
	if env.Code != 200 {
		return nil, &APIError{Op: endpoint, Detail: fmt.Sprintf("application code %d", env.Code)}
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
	Size     *int64          `json:"size"`
	HashInfo json.RawMessage `json:"hash_info"`
	Hashinfo json.RawMessage `json:"hashinfo"`
}

func (r *rawEntry) toEntry() (*Entry, error) {
	if r.Name == "" {
		return nil, &APIError{Op: "entry", Detail: "missing name"}
	}
	if r.IsDir == nil {
		return nil, &APIError{Op: "entry", Detail: "missing is_dir"}
	}
	e := &Entry{Name: r.Name, IsDir: *r.IsDir, Size: r.Size}
	sha, ok, anomaly := decodeSHA1(r.HashInfo, r.Hashinfo)
	e.SHA1, e.HasSHA, e.Anomaly = sha, ok, anomaly
	return e, nil
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

type listData struct {
	Content []rawEntry `json:"content"`
	Total   *int64     `json:"total"`
}

// List walks every page of a remote directory. A directory is only reported
// complete when all pages succeed, the total stays consistent and the final
// page terminates exactly at the declared total.
func (c *Client) List(ctx context.Context, remotePath string) ([]Entry, error) {
	var all []Entry
	seen := map[string]struct{}{}
	var expectedTotal *int64
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
		var parsed listData
		if err := json.Unmarshal(data, &parsed); err != nil {
			return nil, &APIError{Op: "/api/fs/list", Detail: "invalid list shape"}
		}
		if parsed.Total == nil || *parsed.Total < 0 {
			return nil, &APIError{Op: "/api/fs/list", Detail: "missing or invalid total"}
		}
		if expectedTotal == nil {
			expectedTotal = parsed.Total
		} else if *parsed.Total != *expectedTotal {
			return nil, &APIError{Op: "/api/fs/list", Detail: "total changed during traversal"}
		}
		if len(parsed.Content) == 0 {
			if int64(len(all)) < *expectedTotal {
				return nil, &APIError{Op: "/api/fs/list", Detail: "page returned no content before total was reached"}
			}
			break
		}
		if len(parsed.Content) > perPage {
			return nil, &APIError{Op: "/api/fs/list", Detail: "page exceeds per_page"}
		}
		for i := range parsed.Content {
			entry, err := parsed.Content[i].toEntry()
			if err != nil {
				return nil, err
			}
			if _, dup := seen[entry.Name]; dup {
				return nil, &APIError{Op: "/api/fs/list", Detail: "duplicate entry name " + entry.Name}
			}
			seen[entry.Name] = struct{}{}
			all = append(all, *entry)
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

// IsNotFound reports whether an error is a definite remote 404. Callers must
// not treat other failures as absence.
func IsNotFound(err error) bool {
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		return strings.Contains(apiErr.Detail, "404")
	}
	return false
}
