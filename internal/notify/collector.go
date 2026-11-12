package notify

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Rendering and capacity limits fixed by the Task contract. They are not
// configuration items.
const (
	// PreviewMaxEvents bounds the file events shown in the message body.
	PreviewMaxEvents = 20
	// BodyMaxUTF16Units bounds the plain-text message body.
	BodyMaxUTF16Units = 3500
	// AttachmentMaxBytes bounds the single temporary attachment, including its
	// header and trailer lines. It stays below the platform upload limit and
	// leaves headroom for a bounded /tmp.
	AttachmentMaxBytes = 8 * 1024 * 1024
	// attachmentTrailerMargin reserves space for the final trailer lines so a
	// full listing can never push the incompleteness note out of the file.
	attachmentTrailerMargin = 512
)

// kindLabels are the fixed Chinese operation labels used in previews and the
// attachment listing.
var kindLabels = map[Kind]string{
	KindCreated:         "新增",
	KindUpdated:         "更新",
	KindDeleted:         "实际删除",
	KindReclaimed:       "归属回收",
	KindMissingCleared:  "取消待删除",
	KindMissingAdvanced: "缺失计数",
}

// reasonLabels render the fixed qualification reasons.
var reasonLabels = map[string]string{
	ReasonSourceTarget:      "源目标未读到",
	ReasonExcludedExtension: "扩展名排除",
}

// Collector is the optional detail sidecar for one round. It streams events in
// the order they occurred into one private temporary UTF-8 text file, keeps the
// first PreviewMaxEvents events in memory for the message body and tracks the
// totals needed by the summary. Collection failures degrade the notification
// only; they never affect the synchronization round.
type Collector struct {
	file   *os.File
	writer *bufio.Writer

	runLabel  string
	threshold int
	created   time.Time

	bytes       int64
	total       int
	appended    int
	preview     []Event
	anyChange   bool
	limitHit    bool // stopped appending because of the size cap
	broken      bool // a write failed; the listing cannot be trusted
	headerBytes bool
}

// NewCollector creates the private temporary directory (0700) and listing file
// (0600) for one round. The caller must call Close when the round ends.
func NewCollector(runLabel string, threshold int) (*Collector, error) {
	dir, err := os.MkdirTemp("", "strmgen-notify-")
	if err != nil {
		return nil, errors.New("cannot create the notification temporary directory")
	}
	file, err := os.CreateTemp(dir, "strm-events-*.txt")
	if err != nil {
		os.RemoveAll(dir)
		return nil, errors.New("cannot create the notification temporary file")
	}
	if err := file.Chmod(0o600); err != nil {
		file.Close()
		os.RemoveAll(dir)
		return nil, errors.New("cannot restrict the notification temporary file")
	}
	c := &Collector{
		file:      file,
		writer:    bufio.NewWriter(file),
		runLabel:  runLabel,
		threshold: threshold,
		created:   time.Now(),
	}
	if err := c.writeRaw(fmt.Sprintf("STRM 同步事件清单\n运行标识: %s\n", EscapeText(runLabel))); err != nil {
		c.broken = true
	}
	return c, nil
}

// Record appends one event. It never fails and never blocks the round: a write
// failure only degrades the attachment. Events beyond the size cap stop being
// appended while the totals keep counting.
func (c *Collector) Record(event Event) {
	c.total++
	c.anyChange = true
	if len(c.preview) < PreviewMaxEvents {
		c.preview = append(c.preview, event)
	}
	if c.broken || c.limitHit {
		return
	}
	line := c.eventLine(event)
	needed := int64(len(line))
	if c.bytes+needed+attachmentTrailerMargin > AttachmentMaxBytes {
		c.limitHit = true
		return
	}
	if err := c.writeRaw(line); err != nil {
		c.broken = true
		return
	}
	c.appended++
}

// eventLine renders one bounded, escaped single-line record.
func (c *Collector) eventLine(event Event) string {
	label := kindLabels[event.Kind]
	if label == "" {
		label = "事件"
	}
	b := &strings.Builder{}
	fmt.Fprintf(b, "%d 来源 %d %s %s",
		c.total, event.SourceIndex, label, EscapeText(event.RelativePath))
	switch event.Kind {
	case KindDeleted:
		fmt.Fprintf(b, " 删除前计数 %d/阈值 %d", event.PriorCount, c.threshold)
		if reason, ok := reasonLabels[event.Reason]; ok {
			fmt.Fprintf(b, " 原因 %s", reason)
		}
	case KindReclaimed:
		if reason, ok := reasonLabels[event.Reason]; ok {
			fmt.Fprintf(b, " 原因 %s", reason)
		}
	case KindMissingCleared:
		fmt.Fprintf(b, " 计数 %d→0/阈值 %d", event.PriorCount, c.threshold)
	case KindMissingAdvanced:
		fmt.Fprintf(b, " 计数 %d→%d/阈值 %d", event.PriorCount, event.NewCount, c.threshold)
		if reason, ok := reasonLabels[event.Reason]; ok {
			fmt.Fprintf(b, " 原因 %s", reason)
		}
	}
	b.WriteString("\n")
	return b.String()
}

// writeRaw appends bytes to the listing and tracks the size.
func (c *Collector) writeRaw(s string) error {
	n, err := c.writer.WriteString(s)
	c.bytes += int64(n)
	if err != nil {
		return err
	}
	c.headerBytes = true
	return nil
}

// TotalEvents reports the number of valid events observed in this round.
func (c *Collector) TotalEvents() int {
	if c == nil {
		return 0
	}
	return c.total
}

// Preview returns up to PreviewMaxEvents events in the order they occurred.
func (c *Collector) Preview() []Event {
	if c == nil {
		return nil
	}
	return c.preview
}

// HasChange reports whether any reportable change event was observed. Only
// skipped, filtered or retained statistics never set this flag.
func (c *Collector) HasChange() bool {
	if c == nil {
		return false
	}
	return c.anyChange
}

// Degraded reports whether the collected detail is known to be incomplete,
// either because the size cap stopped the listing or a write failed.
func (c *Collector) Degraded() bool {
	if c == nil {
		return false
	}
	return c.broken || c.limitHit
}

// LimitReached reports whether the size cap stopped the listing.
func (c *Collector) LimitReached() bool { return c != nil && c.limitHit }

// WriteFailed reports whether a local listing write failed.
func (c *Collector) WriteFailed() bool { return c != nil && c.broken }

// Finalize flushes and writes the trailer with the totals and the
// incompleteness state. It is called once before the attachment is uploaded.
func (c *Collector) Finalize() error {
	if c == nil || c.file == nil {
		return nil
	}
	if !c.broken {
		trailer := fmt.Sprintf("事件总数: %d（已写入明细 %d 条）\n", c.total, c.appended)
		if c.limitHit {
			trailer += fmt.Sprintf("清单不完整: 达到 %d 字节大小上限，其余 %d 条未写入\n",
				AttachmentMaxBytes, c.total-c.appended)
		}
		if err := c.writeRaw(trailer); err != nil {
			c.broken = true
		}
	}
	if err := c.writer.Flush(); err != nil {
		c.broken = true
	}
	return nil
}

// AttachmentPath returns the temporary listing path when it carries at least
// one usable event and no write failure destroyed the listing.
func (c *Collector) AttachmentPath() (string, bool) {
	if c == nil || c.broken || !c.headerBytes || c.appended == 0 {
		return "", false
	}
	return c.file.Name(), true
}

// Close flushes, closes and removes every temporary artifact of this round. It
// is best effort: a crash may leave the private directory behind, and leftover
// data is never read back or re-sent.
func (c *Collector) Close() {
	if c == nil {
		return
	}
	if c.writer != nil {
		_ = c.writer.Flush()
	}
	if c.file != nil {
		name := c.file.Name()
		_ = c.file.Close()
		dir := filepath.Dir(name)
		_ = os.RemoveAll(dir)
		c.file = nil
		c.writer = nil
	}
}
