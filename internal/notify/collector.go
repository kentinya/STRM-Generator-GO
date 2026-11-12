package notify

import (
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
//
// Missing-count advances are recorded while the commit transaction runs and
// are provisional until it commits: DiscardProvisional removes every
// provisional record so an uncommitted candidate can never be published as
// progress. Memory stays bounded because every record is written through
// immediately.
type Collector struct {
	file   *os.File
	offset int64

	runLabel  string
	threshold int
	created   time.Time

	bytes       int64
	total       int
	appended    int
	details     int
	preview     []Event
	anyChange   bool
	limitHit    bool // stopped appending because of the size cap
	broken      bool // a write failed; the listing cannot be trusted
	headerBytes bool

	// mark snapshots the state before the first provisional record of the
	// current commit attempt so a failed commit can remove them again.
	mark collectorMark
}

type collectorMark struct {
	set       bool
	bytes     int64
	total     int
	appended  int
	preview   int
	anyChange bool
	limitHit  bool
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
	if c == nil {
		return
	}
	c.record(event)
}

// RecordProvisional appends one missing-count advance while the commit
// transaction that applies it is still running. The record is provisional
// until CommitProvisional confirms the commit; DiscardProvisional removes it
// again after a failed or rolled-back commit attempt.
func (c *Collector) RecordProvisional(event Event) {
	if c == nil || c.file == nil {
		return
	}
	if !c.mark.set {
		c.mark = collectorMark{
			set:       true,
			bytes:     c.bytes,
			total:     c.total,
			appended:  c.appended,
			preview:   len(c.preview),
			anyChange: c.anyChange,
			limitHit:  c.limitHit,
		}
	}
	c.record(event)
}

// CommitProvisional confirms every provisional record of the current commit
// attempt as committed progress.
func (c *Collector) CommitProvisional() {
	if c == nil {
		return
	}
	c.mark.set = false
}

// DiscardProvisional removes every provisional record of the current commit
// attempt after the commit failed or rolled back. A listing that already
// failed a write cannot be rewound reliably and stays broken.
func (c *Collector) DiscardProvisional() {
	if c == nil || !c.mark.set {
		return
	}
	m := c.mark
	c.mark.set = false
	if c.broken || c.file == nil {
		return
	}
	if err := c.file.Truncate(m.bytes); err != nil {
		c.broken = true
		return
	}
	if _, err := c.file.Seek(m.bytes, 0); err != nil {
		c.broken = true
		return
	}
	c.bytes = m.bytes
	c.total = m.total
	c.appended = m.appended
	c.preview = c.preview[:m.preview]
	c.anyChange = m.anyChange
	c.limitHit = m.limitHit
	c.offset = m.bytes
}

func (c *Collector) record(event Event) {
	c.total++
	c.anyChange = true
	if len(c.preview) < PreviewMaxEvents {
		c.preview = append(c.preview, event)
	}
	if c.broken || c.limitHit || c.file == nil {
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

// AppendSourceDetail appends one already-escaped source diagnostic line to the
// listing when the message body cannot carry it. Appending stops at the size
// cap and a write failure degrades the attachment only.
func (c *Collector) AppendSourceDetail(line string) {
	if c == nil || c.broken || c.limitHit || c.file == nil {
		return
	}
	needed := int64(len(line))
	if c.bytes+needed+attachmentTrailerMargin > AttachmentMaxBytes {
		c.limitHit = true
		return
	}
	if err := c.writeRaw(line); err != nil {
		c.broken = true
		return
	}
	c.details++
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
		fmt.Fprintf(b, " 删除前计数 %d/阈值 %d 已实际删除", event.PriorCount, c.threshold)
		if reason, ok := reasonLabels[event.Reason]; ok {
			fmt.Fprintf(b, " 原因 %s", reason)
		}
	case KindReclaimed:
		b.WriteString(" 本地目标此前已不存在，归属已移除")
		if reason, ok := reasonLabels[event.Reason]; ok {
			fmt.Fprintf(b, " 原因 %s", reason)
		}
	case KindMissingCleared:
		fmt.Fprintf(b, " 计数 %d→0/阈值 %d 已确认恢复条件，文件保留", event.PriorCount, c.threshold)
	case KindMissingAdvanced:
		fmt.Fprintf(b, " 计数 %d→%d/阈值 %d", event.PriorCount, event.NewCount, c.threshold)
		if reason, ok := reasonLabels[event.Reason]; ok {
			fmt.Fprintf(b, " 原因 %s", reason)
		}
		fmt.Fprintf(b, "；%s", advanceStage(event.NewCount, c.threshold))
	}
	b.WriteString("\n")
	return b.String()
}

// advanceStage describes where one committed count stands in the cleanup
// pipeline. The wording reuses the established contract vocabulary.
func advanceStage(newCount int64, threshold int) string {
	switch {
	case threshold > 0 && newCount >= int64(threshold):
		return "达到阈值，等待后续清理重核，不等于已删除"
	case newCount <= 1:
		return "首次进入待删除，文件仍保留"
	default:
		return "待删除计数推进，文件仍保留"
	}
}

// writeRaw appends bytes to the listing and tracks the size.
func (c *Collector) writeRaw(s string) error {
	n, err := c.file.WriteString(s)
	c.bytes += int64(n)
	c.offset = c.bytes
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
		if c.details > 0 {
			trailer += fmt.Sprintf("来源明细: 追加 %d 条\n", c.details)
		}
		if c.limitHit {
			trailer += fmt.Sprintf("清单不完整: 达到 %d 字节大小上限，其余内容未写入\n",
				AttachmentMaxBytes)
		}
		if err := c.writeRaw(trailer); err != nil {
			c.broken = true
		}
	}
	return nil
}

// AttachmentPath returns the temporary listing path when it carries usable
// content and no write failure destroyed the listing.
func (c *Collector) AttachmentPath() (string, bool) {
	if c == nil || c.broken || !c.headerBytes || (c.appended == 0 && c.details == 0) {
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
	if c.file != nil {
		name := c.file.Name()
		_ = c.file.Close()
		dir := filepath.Dir(name)
		_ = os.RemoveAll(dir)
		c.file = nil
	}
}
