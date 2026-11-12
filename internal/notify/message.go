package notify

import (
	"fmt"
	"strings"
	"time"
)

// Message rendering limits fixed by the Task contract. They are not
// configuration items.
const (
	// tailReserveUTF16Units reserves space for the bounded tail notes so the
	// preview/omission, attachment, degradation and initialization notes are
	// always fully included. The tail is built from fixed strings and small
	// numbers, so it always stays within this reserve.
	tailReserveUTF16Units = 480

	// headMaxUTF16Units bounds the fixed heading. Even a long legal instance
	// name must never push the heading past what the body can hold together
	// with the reserved tail; the display layer budgets it instead of rejecting
	// the configuration value.
	headMaxUTF16Units = BodyMaxUTF16Units - tailReserveUTF16Units

	// collapsePreviewAt is the first preview size rendered inside one
	// expandable block. Five or fewer events stay directly visible.
	collapsePreviewAt = 6

	eventTitleTag     = "<b>📋 文件事件</b>\n"
	blockquoteOpenTag = "<blockquote expandable>\n"
	blockquoteEndTag  = "</blockquote>\n"
)

// Outcome classifies one round for the summary heading. The classification
// only distinguishes what the product already knows: success, source-level
// failure and initialization failure. A source with conflicts is never
// reported as a success.
type Outcome int

const (
	// OutcomeSuccess: every source completed cleanly.
	OutcomeSuccess Outcome = iota
	// OutcomeSourceFailure: one or more sources did not complete cleanly
	// (partial round, errors, conflicts or a failed source round).
	OutcomeSourceFailure
	// OutcomeInitFailure: no report exists because lock resource, output root
	// or database initialization failed. Counts are unavailable, never
	// fabricated as "successfully processed 0".
	OutcomeInitFailure
)

// Stats mirrors the existing per-round report counters. Each item keeps its
// established meaning; failure counts, diagnostic counts and staged missing
// observations are never mixed into these numbers.
type Stats struct {
	Created   int
	Updated   int
	Skipped   int
	Filtered  int
	Deleted   int
	Reclaimed int
	Retained  int
	Conflicts int
	Errors    int
}

// SourceStatus is one bounded per-source line. Notes reuse the existing
// redacted diagnostic vocabulary; they are already sampled and bounded by the
// caller. DetailNotes carries the full sampled diagnostics for the attachment
// listing when the body cannot show them.
type SourceStatus struct {
	Index       int
	Completed   bool
	Retained    int
	Notes       []string
	NotesMore   int
	DetailNotes []string
}

// RunInfo carries the summary inputs assembled from one round.
type RunInfo struct {
	InstanceName      string
	Mode              string // "sync" or "serve"
	Outcome           Outcome
	StartedAt         time.Time
	FinishedAt        time.Time
	SyncDuration      time.Duration
	SourceCount       int
	CompletedSources  int
	IncompleteSources int
	Stats             Stats
	StatsAvailable    bool // false: counts are unavailable (initialization failure)
	// DetailUnavailable records that the detail collection could not be
	// created this round: file events and missing-count progress are unknown
	// and must never be reported as complete or zero.
	DetailUnavailable bool
	Threshold         int
	Sources           []SourceStatus
}

// ShouldSend applies the configured policy. A busy skip and a cancelled round
// never reach this function; the caller filters them first.
func ShouldSend(policy string, outcome Outcome, anyChange bool) bool {
	switch policy {
	case "error":
		return outcome != OutcomeSuccess
	case "change_or_error":
		return outcome != OutcomeSuccess || anyChange
	default: // "all"
		return true
	}
}

// timestamp renders a time with an explicit timezone.
func timestamp(t time.Time) string {
	if t.IsZero() {
		return "不可用"
	}
	return t.Format("2006-01-02T15:04:05Z07:00")
}

// outcomeTitle is the fixed, bold-safe result heading. The four possible
// titles distinguish success, partial completion, all sources incomplete and
// initialization failure; conflicts are already part of the source-failure
// outcome.
func outcomeTitle(info RunInfo) string {
	switch info.Outcome {
	case OutcomeInitFailure:
		return "❌ STRM 同步初始化失败"
	case OutcomeSourceFailure:
		if info.CompletedSources == 0 && info.SourceCount > 0 {
			return "❌ STRM 同步失败（全部来源未完成）"
		}
		return "⚠️ STRM 部分来源未完成"
	default:
		return "✅ STRM 同步完成"
	}
}

// htmlEscaper escapes the three characters Telegram's HTML mode requires.
// Dynamic display values are first passed through EscapeText so control,
// line-break and bidirectional characters cannot inject structure; the HTML
// escape then keeps literal &, < and > from becoming tags.
var htmlEscaper = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")

// htmlText renders one dynamic display value as safe HTML body text.
func htmlText(s string) string {
	return htmlEscaper.Replace(EscapeText(s))
}

// htmlRuneUnits reports the UTF-16 length of one rune after HTML escaping.
func htmlRuneUnits(r rune) int {
	switch r {
	case '&':
		return 5
	case '<', '>':
		return 4
	}
	if r > 0xFFFF {
		return 2
	}
	return 1
}

// fitHTMLText escapes and, when needed, shortens one dynamic display value so
// its complete HTML form fits limit UTF-16 code units. Shortening happens on
// whole escaped runes before HTML escaping, so an entity is never split and an
// astral character is never cut. The caller reserves one additional unit for
// the omission marker.
func fitHTMLText(s string, limit int) (string, bool) {
	if limit <= 0 {
		return "", false
	}
	escaped := EscapeText(s)
	total := 0
	for _, r := range escaped {
		total += htmlRuneUnits(r)
	}
	if total <= limit {
		return htmlEscaper.Replace(escaped), true
	}
	budget := limit - 1 // omission marker
	if budget < 0 {
		budget = 0
	}
	used := 0
	cut := 0
	for i, r := range escaped {
		width := htmlRuneUnits(r)
		if used+width > budget {
			cut = i
			break
		}
		used += width
		cut = i + len(string(r))
	}
	return htmlEscaper.Replace(escaped[:cut]), false
}

// BuildMessage renders the Chinese HTML summary body and decides whether the
// single attachment listing is required. The whole body — heading, source
// details, file event preview, expandable block tags and tail notes — shares
// the BodyMaxUTF16Units budget with at most PreviewMaxEvents preview events;
// truncation never splits a Unicode character or an HTML entity. Source
// details that do not fit the body are appended to the same bounded
// attachment, and the final tail notes plus the attachment decision are only
// fixed after those appends and the listing trailer have actually been
// written, so a failed append or trailer write is reported as an incomplete
// listing instead of promising an unusable attachment.
func BuildMessage(info RunInfo, c *Collector) (text string, attachmentPath string) {
	head := buildHead(info)
	units := buildSourceUnits(info)

	remaining := BodyMaxUTF16Units - UTF16Len(head) - tailReserveUTF16Units
	if remaining < 0 {
		remaining = 0
	}

	// Source units first: they explain why a round failed or what was
	// preserved, before the individual file events. A unit is included whole
	// or omitted whole, so the attachment stays a complete superset.
	shownSources := 0
	var bodyUnits [][]string
	for _, unit := range units {
		needed := linesLen(unit.bodyLines)
		if needed > remaining {
			break
		}
		bodyUnits = append(bodyUnits, unit.bodyLines)
		remaining -= needed
		shownSources++
	}

	shownEvents := 0
	var bodyEvents []string
	if info.Outcome != OutcomeInitFailure {
		for _, event := range c.Preview() {
			if shownEvents >= PreviewMaxEvents {
				break
			}
			candidate := append(append([]string(nil), bodyEvents...), previewLine(event, info.Threshold))
			if UTF16Len(eventSection(candidate)) > remaining {
				break
			}
			bodyEvents = candidate
			shownEvents++
		}
	}

	omittedSources := len(units) - shownSources
	totalEvents := c.TotalEvents()

	// Every source unit that did not fit the body goes into the same bounded
	// attachment, in order, with its full escaped detail lines. The trailer
	// is written before the tail notes are fixed so a final write failure is
	// visible in the body; the collector never writes the trailer twice.
	for _, unit := range units[shownSources:] {
		for _, line := range unit.detailLines {
			c.AppendSourceDetail(line)
		}
	}
	if err := c.Finalize(); err != nil {
		_ = err // the degraded collector state drives the tail notes below
	}

	attachmentPath = attachmentPathFor(info, c, shownEvents, omittedSources)
	tail := buildTail(info, c, shownEvents, totalEvents, omittedSources, attachmentPath)

	body := &strings.Builder{}
	body.WriteString(head)
	for _, lines := range bodyUnits {
		for _, line := range lines {
			body.WriteString(line)
		}
	}
	if len(bodyEvents) > 0 {
		body.WriteString(eventSection(bodyEvents))
	}
	body.WriteString(tail)
	return body.String(), attachmentPath
}

// eventSection renders the preview heading plus either the direct event lines
// or one expandable block. Five or fewer events stay directly visible; six or
// more share a single blockquote. The heading, preview/omission notes and
// degradation warnings always stay outside that block.
func eventSection(lines []string) string {
	if len(lines) == 0 {
		return ""
	}
	b := &strings.Builder{}
	b.WriteString("\n")
	b.WriteString(eventTitleTag)
	collapse := len(lines) >= collapsePreviewAt
	if collapse {
		b.WriteString(blockquoteOpenTag)
	}
	for _, line := range lines {
		b.WriteString(line)
	}
	if collapse {
		b.WriteString(blockquoteEndTag)
	}
	return b.String()
}

// linesLen reports the UTF-16 length of one line group.
func linesLen(lines []string) int {
	n := 0
	for _, line := range lines {
		n += UTF16Len(line)
	}
	return n
}

// buildHead renders the fixed heading, result, statistics, source completion
// and timing sections. Every dynamic value is escaped so a crafted file name or
// instance name can never inject tags or reorder the rendered text. A long
// legal instance name is truncated in the display layer with an explicit
// omission marker so the heading always stays within its budget; no
// configuration value is rejected for being long.
func buildHead(info RunInfo) string {
	overhead := headWithInstance(info, "")
	budget := headMaxUTF16Units - UTF16Len(overhead) - UTF16Len("…")
	if budget < 0 {
		budget = 0
	}
	instance, fits := fitHTMLText(info.InstanceName, budget)
	if !fits {
		instance += "…"
	}
	return headWithInstance(info, instance)
}

// headWithInstance renders the heading with the given already-escaped instance
// display value.
func headWithInstance(info RunInfo, instance string) string {
	b := &strings.Builder{}
	fmt.Fprintf(b, "<b>%s</b>\n", outcomeTitle(info))
	fmt.Fprintf(b, "🏷️ %s · %s\n\n", instance, htmlText(info.Mode))
	if info.Outcome == OutcomeInitFailure || !info.StatsAvailable {
		b.WriteString("统计：不可用\n")
	} else {
		s := info.Stats
		fmt.Fprintf(b, "➕ 新增 %d · ✏️ 更新 %d\n", s.Created, s.Updated)
		fmt.Fprintf(b, "🗑️ 实际删除 %d · ♻️ 归属回收 %d\n", s.Deleted, s.Reclaimed)
		fmt.Fprintf(b, "⏭️ 未变化跳过 %d · 🔎 扩展名过滤 %d\n", s.Skipped, s.Filtered)
		fmt.Fprintf(b, "🛡️ 安全保留 %d · ⚠️ 冲突 %d · ❌ 处理错误 %d\n", s.Retained, s.Conflicts, s.Errors)
	}
	b.WriteString("\n")
	if info.SourceCount > 0 {
		if info.Outcome == OutcomeInitFailure || !info.StatsAvailable {
			// The configured source count is known even without a report;
			// per-source completion is not.
			fmt.Fprintf(b, "📂 来源 %d · 完成/未完成计数不可用\n", info.SourceCount)
		} else {
			fmt.Fprintf(b, "📂 来源 %d · 完成 %d / 未完成 %d\n",
				info.SourceCount, info.CompletedSources, info.IncompleteSources)
		}
	}
	fmt.Fprintf(b, "⏱️ 同步耗时 %s\n", formatDuration(info.SyncDuration))
	if !info.StartedAt.IsZero() {
		fmt.Fprintf(b, "🕒 开始 %s\n", timestamp(info.StartedAt))
	}
	fmt.Fprintf(b, "🕒 完成 %s\n", timestamp(info.FinishedAt))
	return b.String()
}

// buildSourceUnits renders the bounded per-source diagnostic section. The body
// keeps the established five-note sample per source; the attachment carries
// every sampled diagnostic when the whole source unit cannot fit the body.
// Failure and preservation diagnostics are rendered before the file event
// preview and never inside its expandable block. All dynamic content is
// escaped.
func buildSourceUnits(info RunInfo) []sourceUnit {
	if info.Outcome == OutcomeInitFailure {
		return nil
	}
	var units []sourceUnit
	for _, src := range info.Sources {
		var unit sourceUnit
		state := "完成"
		if !src.Completed {
			state = "未完成"
		}
		unit.bodyLines = append(unit.bodyLines, fmt.Sprintf("\n📂 来源 %d · %s\n", src.Index, state))
		unit.detailLines = append(unit.detailLines, fmt.Sprintf("来源 %d：%s\n", src.Index, state))
		for i, note := range src.Notes {
			if i >= 5 {
				break
			}
			unit.bodyLines = append(unit.bodyLines,
				fmt.Sprintf("  来源 %d：%s\n", src.Index, htmlText(note)))
		}
		for _, note := range src.DetailNotes {
			unit.detailLines = append(unit.detailLines, fmt.Sprintf("  来源 %d：%s\n", src.Index, EscapeText(note)))
		}
		if src.NotesMore > 0 {
			more := fmt.Sprintf("  来源 %d：（其余 %d 条诊断省略）\n", src.Index, src.NotesMore)
			unit.bodyLines = append(unit.bodyLines, more)
			unit.detailLines = append(unit.detailLines, more)
		}
		units = append(units, unit)
	}
	return units
}

// sourceUnit is one bounded per-source block. bodyLines fit the message body
// as a whole; detailLines are the attachment superset.
type sourceUnit struct {
	bodyLines   []string
	detailLines []string
}

// kindBodyLabels are the fixed operation labels with their icons for the HTML
// preview. The plain attachment keeps kindLabels.
var kindBodyLabels = map[Kind]string{
	KindCreated:         "➕ 新增",
	KindUpdated:         "✏️ 更新",
	KindDeleted:         "🗑️ 实际删除",
	KindReclaimed:       "♻️ 归属回收",
	KindMissingCleared:  "↩️ 恢复保留",
	KindMissingAdvanced: "⏳ 待删除计数",
}

// previewLine renders one bounded event line for the HTML body preview. Every
// event carries its operation, original source number, complete output-root
// relative path, qualification reason and stage in the cleanup pipeline, so a
// single notification never hides why a file was counted, cleared, kept or
// deleted. The path is escaped as HTML and is never shortened or replaced by a
// basename; an event that does not fit the body is moved wholly to the
// attachment.
func previewLine(event Event, threshold int) string {
	label := kindBodyLabels[event.Kind]
	if label == "" {
		label = "事件"
	}
	b := &strings.Builder{}
	fmt.Fprintf(b, "  %s · 来源 %d · <code>%s</code>",
		label, event.SourceIndex, htmlText(event.RelativePath))
	switch event.Kind {
	case KindDeleted:
		fmt.Fprintf(b, "（删除前计数 %d/阈值 %d，已实际删除", event.PriorCount, threshold)
		if reason, ok := reasonLabels[event.Reason]; ok {
			fmt.Fprintf(b, "，原因 %s", reason)
		}
		b.WriteString("）")
	case KindReclaimed:
		b.WriteString("（本地目标此前已不存在，归属已移除")
		if reason, ok := reasonLabels[event.Reason]; ok {
			fmt.Fprintf(b, "，原因 %s", reason)
		}
		b.WriteString("）")
	case KindMissingCleared:
		fmt.Fprintf(b, "（计数 %d→0/阈值 %d，已确认恢复条件，文件保留）", event.PriorCount, threshold)
	case KindMissingAdvanced:
		fmt.Fprintf(b, "（计数 %d→%d/阈值 %d，", event.PriorCount, event.NewCount, threshold)
		if reason, ok := reasonLabels[event.Reason]; ok {
			fmt.Fprintf(b, "原因 %s，", reason)
		}
		fmt.Fprintf(b, "%s）", advanceStage(event.NewCount, threshold))
	}
	b.WriteString("\n")
	return b.String()
}

// buildTail renders the omission, attachment and incompleteness notes. It is
// always short and always fully included in the body outside the expandable
// event block. The attachment note only promises an upload when a usable
// listing actually exists; a size cap or write failure is reported instead of
// claiming a complete listing that cannot exist.
func buildTail(info RunInfo, c *Collector, shownEvents int, totalEvents int, omittedSources int, attachmentPath string) string {
	if info.Outcome == OutcomeInitFailure {
		return "\n本轮同步初始化失败；未产出可报告的文件与计数事实。\n"
	}
	if info.DetailUnavailable {
		return "\n明细不完整：本轮明细收集不可用，文件事件与缺失计数进度未知。\n"
	}
	b := &strings.Builder{}
	b.WriteString("\n")
	if totalEvents == 0 {
		b.WriteString("文件事件：本轮无新增、更新、删除或归属回收事件。\n")
	} else if totalEvents > shownEvents {
		fmt.Fprintf(b, "正文预览 %d 条，另有 %d 条未展示。\n", shownEvents, totalEvents-shownEvents)
	}
	switch {
	case omittedSources > 0 && c.LimitReached():
		fmt.Fprintf(b, "来源明细：另有 %d 个来源的诊断未在正文展示，附件清单已达大小上限。\n", omittedSources)
	case omittedSources > 0 && c.WriteFailed():
		fmt.Fprintf(b, "明细不完整：本地明细写入失败，另有 %d 个来源的诊断未知。\n", omittedSources)
	case omittedSources > 0:
		fmt.Fprintf(b, "来源明细：另有 %d 个来源的诊断未在正文展示。\n", omittedSources)
	}
	if c.LimitReached() {
		b.WriteString("明细不完整：附件清单达到大小上限，未包含全部事件或来源明细。\n")
	}
	if c.WriteFailed() {
		b.WriteString("明细不完整：本地明细收集写入失败，清单可能缺失。\n")
	}
	if attachmentPath != "" {
		if totalEvents > 0 {
			if c.LimitReached() {
				b.WriteString("📎 清单随附上传（已达大小上限，内容不完整）。\n")
			} else {
				fmt.Fprintf(b, "📎 完整清单 %d 条，随附上传。\n", totalEvents)
			}
		} else {
			b.WriteString("📎 来源明细清单，随附上传。\n")
		}
	} else if (totalEvents > shownEvents || omittedSources > 0) && !c.WriteFailed() && !c.LimitReached() {
		b.WriteString("明细不完整：附件清单不可用。\n")
	}
	return b.String()
}

// attachmentPathFor decides whether the single attachment is required. It is
// required when the body cannot carry every file event or every source detail
// line; the temporary listing must then exist with usable content.
func attachmentPathFor(info RunInfo, c *Collector, shownEvents int, omittedSources int) string {
	if info.Outcome == OutcomeInitFailure {
		return ""
	}
	if info.DetailUnavailable {
		return ""
	}
	total := c.TotalEvents()
	if total <= shownEvents && omittedSources == 0 {
		return ""
	}
	if c.WriteFailed() {
		return ""
	}
	path, ok := c.AttachmentPath()
	if !ok {
		return ""
	}
	return path
}

// formatDuration renders a bounded human-readable duration.
func formatDuration(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	switch {
	case d < time.Second:
		return fmt.Sprintf("%d 毫秒", d.Milliseconds())
	case d < time.Minute:
		return fmt.Sprintf("%.1f 秒", d.Seconds())
	default:
		return fmt.Sprintf("%d 分 %d 秒", int(d.Minutes()), int(d.Seconds())%60)
	}
}
