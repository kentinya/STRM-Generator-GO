package notify

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"os"
	"strings"
	"time"
)

// escapeQuotes escapes the double quotes and backslashes of a multipart
// header value.
func escapeQuotes(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	return strings.ReplaceAll(s, "\"", "\\\"")
}

// Transport limits fixed by the Slice contract. They are not configuration.
const (
	// SendBudget bounds the total time of one summary plus its optional
	// attachment.
	SendBudget = 30 * time.Second
	// maxResponseBytes caps how much of one API response body is read. A
	// Telegram Bot API JSON answer is far smaller; anything larger is cut off
	// without being parsed further.
	maxResponseBytes = 64 * 1024
	// attachmentFieldName is the multipart field name of the document upload.
	attachmentFieldName = "document"
	// attachmentFileName is the declared upload file name of the listing.
	attachmentFileName = "strm-events.txt"
	// attachmentContentType is the declared media type of the UTF-8 listing.
	attachmentContentType = "text/plain; charset=utf-8"
	// parseModeHTML is the fixed Telegram parse mode for the summary. Only
	// the controlled <b>, <code> and <blockquote expandable> tags are emitted.
	parseModeHTML = "HTML"
)

// TelegramAPIBase is the official Bot API origin. Tests can point a Sender at
// a local server through newSenderForBase; production code always uses this
// constant and no configurable production endpoint exists.
const TelegramAPIBase = "https://api.telegram.org"

// Sender delivers one round notification over the official HTTPS Bot API. It
// sends at most one sendMessage and, when needed, one sendDocument upload
// within a single shared time budget. There is no retry (including 429), no
// queue and no exactly-once guarantee: a lost response means the delivery
// result is unknown.
type Sender struct {
	token   string
	chatID  string
	apiBase string
	client  *http.Client
}

// NewSender builds a sender for the official Bot API. TLS verification stays
// enabled; redirects are never followed.
func NewSender(token, chatID string) *Sender {
	return NewSenderWithBase(token, chatID, TelegramAPIBase)
}

// NewSenderWithBase builds a sender against an explicit API origin. Production
// callers pass TelegramAPIBase; only tests and the testhooks binary use
// another value.
func NewSenderWithBase(token, chatID, apiBase string) *Sender {
	return &Sender{
		token:   token,
		chatID:  chatID,
		apiBase: apiBase,
		client: &http.Client{
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}
}

// apiError is the fixed, redacted failure vocabulary. It never contains the
// request address, the token, the chat ID, the response description or any
// raw network error.
var (
	errRequestFailed    = errors.New("request failed")
	errRequestRejected  = errors.New("request was rejected")
	errResponseInvalid  = errors.New("response was not a valid result")
	errAttachmentFailed = errors.New("attachment upload failed")
)

// Send posts the HTML summary and, when attachmentPath is non-empty, uploads
// the single UTF-8 listing. Both share the fixed budget. Sending
// stops immediately when ctx is cancelled. The returned error is always a
// redacted, fixed-vocabulary error.
func (s *Sender) Send(ctx context.Context, text string, attachmentPath string) error {
	ctx, cancel := context.WithTimeout(ctx, SendBudget)
	defer cancel()

	if err := s.sendMessage(ctx, text); err != nil {
		return err
	}
	if attachmentPath == "" {
		return nil
	}
	// The summary succeeded; the attachment is one best-effort upload. A
	// failure is never followed by a compensating status message.
	if err := s.sendDocument(ctx, attachmentPath); err != nil {
		return fmt.Errorf("%w: %w", errAttachmentFailed, err)
	}
	return nil
}

// call performs one bounded POST. The response body is capped, redirects are
// rejected by the client and the status is checked before parsing.
func (s *Sender) call(ctx context.Context, method string, contentType string, body io.Reader) error {
	endpoint := s.apiBase + "/bot" + s.token + "/" + method
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, body)
	if err != nil {
		return errRequestFailed
	}
	req.Header.Set("Content-Type", contentType)
	resp, err := s.client.Do(req)
	if err != nil {
		// Cancellation keeps its identity so the caller can tell it apart.
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return errRequestFailed
	}
	defer func() {
		io.Copy(io.Discard, io.LimitReader(resp.Body, maxResponseBytes))
		resp.Body.Close()
	}()
	if resp.StatusCode == http.StatusSwitchingProtocols || resp.StatusCode >= 300 || resp.StatusCode < 200 {
		// Includes rejected redirects: the client surfaces the 3xx response
		// without following it.
		return fmt.Errorf("%w (status %d)", errRequestRejected, resp.StatusCode)
	}
	limited := io.LimitReader(resp.Body, maxResponseBytes)
	payload, err := io.ReadAll(limited)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return errResponseInvalid
	}
	var parsed struct {
		OK bool `json:"ok"`
	}
	if err := json.Unmarshal(payload, &parsed); err != nil {
		return errResponseInvalid
	}
	if !parsed.OK {
		return errRequestRejected
	}
	return nil
}

func (s *Sender) sendMessage(ctx context.Context, text string) error {
	form := url.Values{}
	form.Set("chat_id", s.chatID)
	form.Set("parse_mode", parseModeHTML)
	form.Set("text", text)
	return s.call(ctx, "sendMessage", "application/x-www-form-urlencoded", strings.NewReader(form.Encode()))
}

func (s *Sender) sendDocument(ctx context.Context, path string) error {
	file, err := os.Open(path)
	if err != nil {
		return errAttachmentFailed
	}
	defer file.Close()

	pr, pw := io.Pipe()
	writer := multipart.NewWriter(pw)
	go func() {
		var firstErr error
		defer func() {
			pw.CloseWithError(firstErr)
		}()
		if err := writer.WriteField("chat_id", s.chatID); err != nil && firstErr == nil {
			firstErr = err
			return
		}
		header := make(textproto.MIMEHeader)
		header.Set("Content-Disposition",
			fmt.Sprintf(`form-data; name="%s"; filename="%s"`,
				escapeQuotes(attachmentFieldName), escapeQuotes(attachmentFileName)))
		header.Set("Content-Type", attachmentContentType)
		part, err := writer.CreatePart(header)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			return
		}
		if _, err := io.Copy(part, file); err != nil && firstErr == nil {
			firstErr = err
			return
		}
		if err := writer.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}()

	return s.call(ctx, "sendDocument", writer.FormDataContentType(), pr)
}
