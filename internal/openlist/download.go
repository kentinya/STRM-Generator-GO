package openlist

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net"
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
	// formats never start with '<' or '{' (a UTF-8 mark and blanks are
	// skipped first); a '[' body is only an error page when the complete
	// response is valid JSON, because an ASS subtitle starts with a section
	// header.
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
// client, while the download is an unauthenticated HTTP(S) GET. Every round
// builds exactly one downloader, so its pacer and transport are shared by all
// sources and directories of that round and never persist across rounds.
type SubtitleDownloader struct {
	publicPrefix string
	http         *http.Client
	transport    *http.Transport
	pacer        *downloadPacer
}

// newSubtitleTransport builds the dedicated subtitle transport. It never
// touches http.DefaultTransport or any other client: the subtitle feature has
// its own connection pool whose lifetime is bounded by one round. The HTTP
// protocol set is configured explicitly on this fixed Go toolchain, so the
// negotiation layer really changes: with h2 disabled the transport allows
// HTTP/1 only and the TLS handshake carries no ALPN protocol offer, so an
// HTTPS service that also supports HTTP/2 still negotiates HTTP/1.1; plain
// HTTP stays HTTP/1.1 and h2c is never attempted. With h2 enabled the
// transport explicitly allows HTTP/2 over TLS while HTTP/1.1 stays available
// as the fallback for HTTP/1.1-only services.
func newSubtitleTransport(allowHTTP2 bool) *http.Transport {
	t := &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		DialContext:           (&net.Dialer{Timeout: 30 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
		MaxIdleConns:          16,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: time.Second,
	}
	t.Protocols = new(http.Protocols)
	t.Protocols.SetHTTP1(true)
	if allowHTTP2 {
		t.Protocols.SetHTTP2(true)
	}
	return t
}

// NewSubtitleDownloader builds a downloader for the validated public prefix.
// interval is the minimum start-to-start spacing between two subtitle
// download attempts of the same round; the configuration layer guarantees a
// value of at least 0.5 seconds. allowHTTP2=false keeps the whole redirect
// chain on HTTP/1.1 even when the HTTPS service supports HTTP/2.
func NewSubtitleDownloader(publicPrefix string, interval time.Duration, allowHTTP2 bool) *SubtitleDownloader {
	transport := newSubtitleTransport(allowHTTP2)
	return &SubtitleDownloader{
		publicPrefix: publicPrefix,
		transport:    transport,
		http: &http.Client{
			Transport: transport,
			// Redirects are followed manually so the count and the scheme stay
			// under this package's control instead of the transport default.
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
		pacer: &downloadPacer{interval: interval},
	}
}

// WithHTTPClient replaces the underlying HTTP client (used by tests). The
// pacer and the transport stay untouched so the pacing contract still applies
// to the replaced client.
func (d *SubtitleDownloader) WithHTTPClient(hc *http.Client) *SubtitleDownloader {
	hc.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}
	d.http = hc
	return d
}

// WithRootCAs replaces the transport root CA pool. It exists only for tests
// against local TLS servers and never touches the protocol configuration, so
// a test that injects trust anchors still exercises the production protocol
// and pacing path.
func (d *SubtitleDownloader) WithRootCAs(pool *x509.CertPool) *SubtitleDownloader {
	if d.transport.TLSClientConfig == nil {
		d.transport.TLSClientConfig = &tls.Config{RootCAs: pool}
		return d
	}
	d.transport.TLSClientConfig.RootCAs = pool
	return d
}

// CloseIdleConnections releases the idle connections of the dedicated
// subtitle transport. runRound calls it after the round so a long-lived serve
// process never accumulates idle connections from finished rounds.
func (d *SubtitleDownloader) CloseIdleConnections() {
	d.transport.CloseIdleConnections()
}

// Fetch streams exactly one subtitle file into w. It first waits for the
// round-shared download slot: the first subtitle starts immediately and the
// next one starts at least interval after the previous attempt started, with
// no burst catch-up after an idle period or a fast download. The 30-second
// transfer budget starts only after the wait, so it still covers every
// redirect hop and the body read but never the wait itself.
func (d *SubtitleDownloader) Fetch(ctx context.Context, remotePath string, w io.Writer) error {
	target, err := pathmap.StableURL(d.publicPrefix, remotePath)
	if err != nil {
		return errSubtitleURL
	}
	if err := d.pacer.wait(ctx); err != nil {
		// A cancelled wait never issues a request and never publishes a file.
		return errSubtitleTransport
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

// downloadPacer spaces the subtitle download attempts of one round. last is
// the monotonic start time of the most recent actual attempt (success or
// failure); it is never advanced by skips, filtered entries or local failures
// that issued no content request, and it never accumulates credit: after a
// long idle period the next attempt starts immediately.
type downloadPacer struct {
	interval time.Duration
	// now is the monotonic clock source; nil keeps time.Now. Tests replace it
	// to keep the scheduling math deterministic without real sleeps.
	now func() time.Time
	// sleep blocks until the delay elapses or ctx ends; nil keeps the real
	// cancellable timer. Tests replace it to observe the computed delay.
	sleep func(ctx context.Context, delay time.Duration) error
	last  time.Time
}

// realSleep waits on a real timer that a round cancellation interrupts. The
// wait is part of neither the 30-second budget nor any request.
func realSleep(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// wait blocks until the next attempt may start and records that start.
// The wait is cancellable: a cancelled wait returns without recording an
// attempt and without issuing a request.
func (p *downloadPacer) wait(ctx context.Context) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if p == nil || p.interval <= 0 {
		return nil
	}
	nowFn := p.now
	if nowFn == nil {
		nowFn = time.Now
	}
	now := nowFn()
	if !p.last.IsZero() {
		if delay := p.interval - now.Sub(p.last); delay > 0 {
			sleep := p.sleep
			if sleep == nil {
				sleep = realSleep
			}
			if err := sleep(ctx, delay); err != nil {
				return err
			}
			now = nowFn()
		}
	}
	p.last = now
	return nil
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

// utf8BOM is the UTF-8 byte order mark. An error page saved by a text editor
// may carry one and so may a legal subtitle, so the mark is skipped while
// classifying a body and never removed from the stored bytes.
var utf8BOM = []byte{0xEF, 0xBB, 0xBF}

// stripBodyPrefix removes leading blanks and every complete UTF-8 mark from a
// body prefix so classification looks at the first meaningful byte. An
// incomplete mark at the end of the prefix is left untouched: the caller
// decides with the bytes it already has.
func stripBodyPrefix(b []byte) []byte {
	for {
		b = bytes.TrimLeft(b, " \t\r\n")
		if bytes.HasPrefix(b, utf8BOM) {
			b = b[len(utf8BOM):]
			continue
		}
		return b
	}
}

// sniffSignature returns the first meaningful byte of the window and reports
// whether the window already settles the signature: a non-blank byte after
// the blanks and marks, or a window that is full of blanks. A window that
// ends inside a byte order mark settles nothing, because the next bytes
// decide whether the mark is complete.
func sniffSignature(window []byte) (byte, bool) {
	for {
		trimmed := bytes.TrimLeft(window, " \t\r\n")
		if bytes.HasPrefix(trimmed, utf8BOM) {
			window = trimmed[len(utf8BOM):]
			continue
		}
		if len(trimmed) == 0 {
			if len(window) >= errorPageSniffBytes {
				return 0, true
			}
			return 0, false
		}
		if len(trimmed) < len(utf8BOM) && bytes.HasPrefix(utf8BOM, trimmed) {
			return 0, false
		}
		return trimmed[0], true
	}
}

// isErrorPage reports whether the streamed body is an HTML or JSON error
// page. Blanks and a UTF-8 mark are skipped first, so a BOM'd error page is
// classified like any other; '<' and '{' can only be error documents in the
// subtitle whitelist, and a '[' body must be a complete JSON document to
// count, since a legal ASS file starts with a section header.
func (s *sniffWriter) isErrorPage() bool {
	body := s.prefix
	if s.mirror != nil {
		body = make([]byte, 0, len(s.prefix)+s.mirror.Len())
		body = append(body, s.prefix...)
		body = append(body, s.mirror.Bytes()...)
	}
	trimmed := stripBodyPrefix(body)
	if len(trimmed) == 0 {
		return false
	}
	switch trimmed[0] {
	case '<', '{':
		return true
	case '[':
		return json.Valid(trimmed)
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
