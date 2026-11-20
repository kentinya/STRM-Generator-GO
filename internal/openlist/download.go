package openlist

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strings"
	"time"

	"strm-generator-go/internal/pathmap"
)

// Subtitle transfer limits. Every value is part of the subtitle contract: one
// 30-second budget covers the redirects and the response body of one file, at
// most five HTTP(S) redirects are followed and one complete HTTP 200 response
// may carry at most 8 MiB.
const (
	// SubtitleDownloadBudget bounds one whole subtitle download, including
	// every redirect hop and the streamed body read.
	SubtitleDownloadBudget = 30 * time.Second
	// SubtitleMaxRedirects is the highest number of redirects one download may
	// follow; the sixth hop fails the download.
	SubtitleMaxRedirects = 5
	// SubtitleMaxBytes bounds the accepted response body of one subtitle.
	SubtitleMaxBytes = int64(8 << 20)
	// errorPageSniffBytes is the leading window of a streamed body inspected
	// when the response content type claims something harmless. Subtitle
	// formats never start with '<' or '{'; a '[' body is only an error page
	// when the complete response is valid JSON, because an ASS subtitle starts
	// with a section header.
	errorPageSniffBytes = 512
)

// errSubtitle gives every download failure a fixed, non-secret reason. The
// caller reports the relative path; the reason never echoes the URL, host or
// response body.
var (
	errSubtitleURL          = errors.New("subtitle download address is invalid")
	errSubtitleTransport    = errors.New("subtitle download transport failed")
	errSubtitleRedirects    = errors.New("subtitle download exceeded the redirect limit")
	errSubtitleRedirectType = errors.New("subtitle redirect target is not http or https")
	errSubtitleStatus       = errors.New("subtitle download status is not 200")
	errSubtitleErrorPage    = errors.New("subtitle response is an html or json error page")
	errSubtitleTooLarge     = errors.New("subtitle response exceeds the byte limit")
	errSubtitleTruncated    = errors.New("subtitle response was truncated")
)

// errSubtitleLimitReached is the internal signal that the streamed body passed
// the accepted byte ceiling; the read classifies it as an oversized response
// instead of a transport failure.
var errSubtitleLimitReached = errors.New("subtitle byte limit reached")

// SubtitleDownloader fetches one subtitle file over the stable public /d/
// address. It never sends the OpenList token: metadata keeps using the API
// client, while the download is an unauthenticated HTTP(S) GET.
type SubtitleDownloader struct {
	publicPrefix string
	http         *http.Client
}

// NewSubtitleDownloader builds a downloader for the validated public prefix.
func NewSubtitleDownloader(publicPrefix string) *SubtitleDownloader {
	return &SubtitleDownloader{
		publicPrefix: publicPrefix,
		http: &http.Client{
			// Redirects are followed manually so the count and the scheme stay
			// under this package's control instead of the transport default.
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}
}

// WithHTTPClient replaces the underlying HTTP client (used by tests).
func (d *SubtitleDownloader) WithHTTPClient(hc *http.Client) *SubtitleDownloader {
	hc.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}
	d.http = hc
	return d
}

// Fetch streams exactly one subtitle file into w. It returns only after the
// complete body was copied or a failure was classified; a partial write never
// reaches a published file because the caller stages into a temporary name.
func (d *SubtitleDownloader) Fetch(ctx context.Context, remotePath string, w io.Writer) error {
	target, err := pathmap.StableURL(d.publicPrefix, remotePath)
	if err != nil {
		return errSubtitleURL
	}
	// One budget covers the redirects and the body read of this file.
	fetchCtx, cancel := context.WithTimeout(ctx, SubtitleDownloadBudget)
	defer cancel()

	redirects := 0
	for {
		if err := fetchCtx.Err(); err != nil {
			return errSubtitleTransport
		}
		req, err := http.NewRequestWithContext(fetchCtx, http.MethodGet, target, nil)
		if err != nil {
			return errSubtitleURL
		}
		resp, err := d.http.Do(req)
		if err != nil {
			return errSubtitleTransport
		}
		if isRedirect(resp.StatusCode) {
			location := resp.Header.Get("Location")
			resp.Body.Close()
			if location == "" {
				return errSubtitleStatus
			}
			if redirects >= SubtitleMaxRedirects {
				return errSubtitleRedirects
			}
			next, err := resolveRedirect(target, location)
			if err != nil {
				return errSubtitleRedirectType
			}
			redirects++
			target = next
			continue
		}
		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			return errSubtitleStatus
		}
		return readSubtitleBody(resp, w)
	}
}

func isRedirect(status int) bool {
	switch status {
	case http.StatusMovedPermanently, http.StatusFound, http.StatusSeeOther,
		http.StatusTemporaryRedirect, http.StatusPermanentRedirect:
		return true
	}
	return false
}

// resolveRedirect resolves one redirect target against the current address
// and accepts only an http(s) location.
func resolveRedirect(current, location string) (string, error) {
	base, err := url.Parse(current)
	if err != nil {
		return "", err
	}
	next, err := base.Parse(location)
	if err != nil {
		return "", err
	}
	if next.Scheme != "http" && next.Scheme != "https" {
		return "", errors.New("non-http redirect")
	}
	return next.String(), nil
}

// readSubtitleBody streams one accepted 200 response into w. An HTML or JSON
// content type is an error page, a declared length above the limit is
// refused before any byte is stored, and the copy checks both the actual byte
// count and read errors so a truncated or oversized body never completes.
// The content type alone is not trusted: an error body answered as text/plain
// is classified from the streamed bytes before the caller can publish them,
// while a valid subtitle keeps its exact bytes.
func readSubtitleBody(resp *http.Response, w io.Writer) error {
	defer resp.Body.Close()
	if isErrorPageContentType(resp.Header.Get("Content-Type")) {
		return errSubtitleErrorPage
	}
	if resp.ContentLength > SubtitleMaxBytes {
		return errSubtitleTooLarge
	}
	sniff := &sniffWriter{w: w}
	// One extra byte is admitted so an oversized body is detected instead of
	// silently cut at the limit.
	limited := &countingLimitWriter{w: sniff, remaining: SubtitleMaxBytes + 1}
	written, err := io.Copy(limited, resp.Body)
	if written > SubtitleMaxBytes {
		return errSubtitleTooLarge
	}
	if errors.Is(err, errSubtitleLimitReached) {
		return errSubtitleTooLarge
	}
	if err != nil {
		// The HTTP client surfaces a cut-off identity body as a read error.
		// A copy that stayed short of the declared length is a truncated
		// response; anything else is a transport failure.
		if resp.ContentLength >= 0 && written < resp.ContentLength {
			return errSubtitleTruncated
		}
		return errSubtitleTransport
	}
	if resp.ContentLength >= 0 && written < resp.ContentLength {
		return errSubtitleTruncated
	}
	if sniff.isErrorPage() {
		return errSubtitleErrorPage
	}
	return nil
}

// sniffWriter streams every byte to the destination while keeping just enough
// of the body to recognise an error page after the copy. The first window
// settles the leading byte; a '[' prefix additionally mirrors the remaining
// bytes so a complete JSON document can be told apart from an ASS section
// header. The mirrored copy exists only for that ambiguous prefix and stays
// bounded by the subtitle byte limit.
type sniffWriter struct {
	w       io.Writer
	prefix  []byte
	mirror  *bytes.Buffer
	settled bool
}

func (s *sniffWriter) Write(p []byte) (int, error) {
	n, err := s.w.Write(p)
	if n > 0 {
		chunk := p[:n]
		if !s.settled && len(s.prefix) < errorPageSniffBytes {
			take := chunk
			if free := errorPageSniffBytes - len(s.prefix); len(take) > free {
				take = take[:free]
			}
			s.prefix = append(s.prefix, take...)
			chunk = chunk[len(take):]
			if signature, ready := sniffSignature(s.prefix); ready {
				s.settled = true
				if signature == '[' {
					s.mirror = &bytes.Buffer{}
				}
			}
		}
		if s.mirror != nil {
			_, _ = s.mirror.Write(chunk)
		}
	}
	return n, err
}

// sniffSignature returns the first non-space byte of the window and reports
// whether the window already settles the signature: either a non-blank byte
// was found or the window is full of blanks.
func sniffSignature(window []byte) (byte, bool) {
	if trimmed := bytes.TrimLeft(window, " \t\r\n"); len(trimmed) > 0 {
		return trimmed[0], true
	}
	if len(window) >= errorPageSniffBytes {
		return 0, true
	}
	return 0, false
}

// isErrorPage reports whether the streamed body is an HTML or JSON error
// page. '<' and '{' can only be error documents in the subtitle whitelist;
// a '[' body must be a complete JSON document to count, since a legal ASS
// file starts with a section header.
func (s *sniffWriter) isErrorPage() bool {
	body := s.prefix
	if s.mirror != nil {
		body = make([]byte, 0, len(s.prefix)+s.mirror.Len())
		body = append(body, s.prefix...)
		body = append(body, s.mirror.Bytes()...)
	}
	trimmed := bytes.TrimLeft(body, " \t\r\n")
	if len(trimmed) == 0 {
		return false
	}
	switch trimmed[0] {
	case '<', '{':
		return true
	case '[':
		return json.Valid(body)
	}
	return false
}

// isErrorPageContentType classifies a 200 response whose content type marks
// an HTML or JSON document. Those are OpenList/web error pages, never subtitle
// bytes. Subtitle files arrive as plain text, octet streams or similar.
func isErrorPageContentType(header string) bool {
	mediaType, _, err := mime.ParseMediaType(strings.TrimSpace(header))
	if err != nil {
		return false
	}
	if mediaType == "text/html" {
		return true
	}
	if mediaType == "application/json" || strings.HasSuffix(mediaType, "+json") {
		return true
	}
	return false
}

// countingLimitWriter bounds the bytes handed to the caller's writer while
// counting everything the response delivered.
type countingLimitWriter struct {
	w         io.Writer
	remaining int64
}

func (c *countingLimitWriter) Write(p []byte) (int, error) {
	if c.remaining <= 0 {
		return 0, errSubtitleLimitReached
	}
	if int64(len(p)) > c.remaining {
		p = p[:c.remaining]
	}
	n, err := c.w.Write(p)
	c.remaining -= int64(n)
	if err != nil {
		return n, err
	}
	if n < len(p) {
		return n, errSubtitleLimitReached
	}
	return n, nil
}
