package notify

import (
	"fmt"
	"strings"
	"time"
)

// tailReserveUTF16Units reserves space for the bounded tail notes so the
// omission and completeness notes are always fully included. The tail is
// built from fixed strings and small numbers, so it always stays within this
// reserve.
const tailReserveUTF16Units = 220

// headMaxUTF16Units bounds the fixed heading. Even a long legal instance name
// must never push the heading past what the body can hold together with the
// reserved tail; the display layer budgets it instead of rejecting the
// configuration value.
const headMaxUTF16Units = BodyMaxUTF16Units - tailReserveUTF16Units

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

func outcomeLabel(info RunInfo) string {
	switch info.Outcome {
	case OutcomeInitFailure:
		return "同步初始化失败"
	case OutcomeSourceFailure:
		if info.CompletedSources == 0 && info.SourceCount > 0 {
			return "同步失败（全部来源未完成）"
		}
		return "部分来源未完成"
	default:
		return "成功"
	}
}

// BuildMessage renders the Chinese plain-text summary body and decides whether
// the single attachment listing is required. The whole body — heading, source
// details, file event preview and tail notes — shares the BodyMaxUTF16Units
// budget with at most PreviewMaxEvents preview events; truncation never splits
// a Unicode character. Source details that do not fit the body are appended to
// the same bounded attachment, and the final tail notes plus the attachment
// decision are only fixed after those appends and the listing trailer have
// actually been written, so a failed append or trailer write is reported as
// an incomplete listing instead of promising an unusable attachment.
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
		needed := 0
		for _, line := range unit.bodyLines {
			needed += UTF16Len(line)
		}
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
			line := previewLine(event, info.Threshold)
			units := UTF16Len(line)
			if units > remaining {
				break
			}
			bodyEvents = append(bodyEvents, line)
			remaining -= units
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

	tail := buildTail(info, c, shownEvents, totalEvents, shownSources, omittedSources)
	// Defensive final budget check. The heading and the tail are bounded by
	// construction, so this loop can only ever drop preview events — and
	// every event is already carried by the listing file, so nothing is lost
	// from the attachment.
	for bodyUnitsLen(bodyUnits)+eventsLen(bodyEvents) > 0 &&
		UTF16Len(head)+bodyUnitsLen(bodyUnits)+eventsLen(bodyEvents)+UTF16Len(tail) > BodyMaxUTF16Units {
		if len(bodyEvents) > 0 {
			bodyEvents = bodyEvents[:len(bodyEvents)-1]
			shownEvents--
		} else {
			bodyUnits = bodyUnits[:len(bodyUnits)-1]
			shownSources--
		}
		omittedSources = len(units) - shownSources
		tail = buildTail(info, c, shownEvents, totalEvents, shownSources, omittedSources)
	}

	body := &strings.Builder{}
	body.WriteString(head)
	for _, lines := range bodyUnits {
		for _, line := range lines {
			body.WriteString(line)
		}
	}
	if len(bodyEvents) > 0 {
		body.WriteString("\n文件事件：\n")
		for _, line := range bodyEvents {
			body.WriteString(line)
		}
	}
	body.WriteString(tail)
	return body.String(), attachmentPathFor(info, c, shownEvents, omittedSources)
}

// bodyUnitsLen reports the UTF-16 length of the selected source unit lines.
func bodyUnitsLen(units [][]string) int {
	n := 0
	for _, lines := range units {
		n += linesLen(lines)
	}
	return n
}

// eventsLen reports the UTF-16 length of the selected preview lines including
// their shared heading.
func eventsLen(lines []string) int {
	if len(lines) == 0 {
		return 0
	}
	return UTF16Len("\n文件事件：\n") + linesLen(lines)
}

// linesLen reports the UTF-16 length of one line group.
func linesLen(lines []string) int {
	n := 0
	for _, line := range lines {
		n += UTF16Len(line)
	}
	return n
}

// buildHead renders the fixed heading, result, timing and statistics sections.
// Every dynamic value is escaped so a crafted file name or instance name can
// never inject lines or reorder the rendered text. A long legal instance name
// is truncated in the display layer with an explicit omission marker so the
// heading always stays within its budget; no configuration value is rejected
// for being long.
func buildHead(info RunInfo) string {
	instance := EscapeText(info.InstanceName)
	if n := UTF16Len(headWithInstance(info, instance)); n > headMaxUTF16Units {
		overhead := UTF16Len(headWithInstance(info, ""))
		budget := headMaxUTF16Units - overhead - UTF16Len("…")
		if budget < 0 {
			budget = 0
		}
		instance = TruncateUTF16(instance, budget) + "…"
	}
	return headWithInstance(info, instance)
}

// headWithInstance renders the heading with the given already-escaped instance
// display value.
func headWithInstance(info RunInfo, instance string) string {
	b := &strings.Builder{}
	fmt.Fprintf(b, "%s STRM 同步报告\n", instance)
	fmt.Fprintf(b, "运行方式：%s\n", info.Mode)
	fmt.Fprintf(b, "结果：%s\n", outcomeLabel(info))
	fmt.Fprintf(b, "完成时间：%s\n", timestamp(info.FinishedAt))
	fmt.Fprintf(b, "同步耗时：%s\n", formatDuration(info.SyncDuration))
	if !info.StartedAt.IsZero() {
		fmt.Fprintf(b, "开始时间：%s\n", timestamp(info.StartedAt))
	}
	if info.SourceCount > 0 {
		if info.Outcome == OutcomeInitFailure || !info.StatsAvailable {
			// The configured source count is known even without a report;
			// per-source completion is not.
			fmt.Fprintf(b, "来源总数：%d（完成/未完成计数不可用）\n", info.SourceCount)
		} else {
			fmt.Fprintf(b, "来源总数：%d（完成 %d / 未完成 %d）\n",
				info.SourceCount, info.CompletedSources, info.IncompleteSources)
		}
	}
	if info.Outcome == OutcomeInitFailure || !info.StatsAvailable {
		b.WriteString("统计：不可用\n")
		return b.String()
	}
	s := info.Stats
	fmt.Fprintf(b, "统计：新增 %d · 更新 %d · 未变化跳过 %d · 扩展名过滤 %d · 实际删除 %d · 归属回收 %d · 安全保留 %d · 冲突 %d · 处理错误 %d\n",
		s.Created, s.Updated, s.Skipped, s.Filtered, s.Deleted, s.Reclaimed, s.Retained, s.Conflicts, s.Errors)
	return b.String()
}

// buildSourceUnits renders the bounded per-source section. The body keeps the
// established five-note sample per source; the attachment carries every
// sampled diagnostic. All dynamic content is escaped.
func buildSourceUnits(info RunInfo) []sourceUnit {
	if info.Outcome == OutcomeInitFailure {
		return nil
	}
	var units []sourceUnit
	for _, src := range info.Sources {
		var unit sourceUnit
		var header string
		if src.Completed {
			header = fmt.Sprintf("来源 %d：完成\n", src.Index)
		} else {
			header = fmt.Sprintf("来源 %d：未完成\n", src.Index)
		}
		unit.bodyLines = append(unit.bodyLines, header)
		unit.detailLines = append(unit.detailLines, header)
		for i, note := range src.Notes {
			if i >= 5 {
				break
			}
			unit.bodyLines = append(unit.bodyLines, fmt.Sprintf("  来源 %d：%s\n", src.Index, EscapeText(note)))
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

// previewLine renders one bounded event line for the body preview. Every
// event carries its qualification reason and its stage in the cleanup
// pipeline, so a single notification never hides why a file was counted,
// cleared, kept or deleted.
func previewLine(event Event, threshold int) string {
	label := kindLabels[event.Kind]
	if label == "" {
		label = "事件"
	}
	b := &strings.Builder{}
	fmt.Fprintf(b, "  %s 来源 %d %s", label, event.SourceIndex, EscapeText(event.RelativePath))
	switch event.Kind {
	case KindDeleted:
		fmt.Fprintf(b, "（删除前计数 %d/阈值 %d，已实际删除", event.PriorCount, threshold)
		if reason, ok := reasonLabels[event.Reason]; ok {
			fmt.Fprintf(b, "，原因 %s", reason)
		}
		b.WriteString("）")
	case KindReclaimed:
		b.WriteString("（本地目标此前已不存在，归属已移除）")
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

// buildTail renders the omission and completeness notes. It is always short
// and always fully included in the body.
func buildTail(info RunInfo, c *Collector, shownEvents int, totalEvents int, shownSources int, omittedSources int) string {
	b := &strings.Builder{}
	if info.Outcome == OutcomeInitFailure {
		b.WriteString("本轮同步初始化失败；未产出可报告的文件与计数事实。\n")
		return b.String()
	}
	if info.DetailUnavailable {
		b.WriteString("明细不完整：本轮明细收集不可用，文件事件与缺失计数进度未知。\n")
		return b.String()
	}
	if totalEvents == 0 {
		b.WriteString("文件事件：本轮无新增、更新、删除或归属回收事件。\n")
	} else if totalEvents > shownEvents {
		b.WriteString(fmt.Sprintf("文件事件：正文仅预览前 %d 条，另有 %d 条未在正文展示。\n", shownEvents, totalEvents-shownEvents))
	}
	// The omission note only promises the attachment when the listing is
	// actually usable; a size cap or a write failure is reported instead of
	// claiming a complete listing that cannot exist.
	switch {
	case omittedSources > 0 && c.LimitReached():
		b.WriteString(fmt.Sprintf("来源明细：另有 %d 条未在正文展示；附件清单已达大小上限，其余来源明细未写入。\n", omittedSources))
	case omittedSources > 0 && c.WriteFailed():
		b.WriteString(fmt.Sprintf("明细不完整：本地明细写入失败，另有 %d 条来源明细未知。\n", omittedSources))
	case omittedSources > 0:
		b.WriteString(fmt.Sprintf("来源明细：另有 %d 条未在正文展示，完整内容见附件清单。\n", omittedSources))
	}
	if c.LimitReached() {
		b.WriteString("明细不完整：附件清单达到大小上限，未包含全部事件。\n")
	}
	if c.WriteFailed() {
		b.WriteString("明细不完整：本地明细收集写入失败，清单可能缺失。\n")
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
