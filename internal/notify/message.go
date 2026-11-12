package notify

import (
	"fmt"
	"strings"
	"time"
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

// SourceStatus is one bounded per-source line for failed sources. Notes reuse
// the existing redacted diagnostic vocabulary; notes are already sampled and
// bounded by the caller.
type SourceStatus struct {
	Index     int
	Completed bool
	Notes     []string
	NotesMore int
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
// the single attachment listing is required. The body is bounded to
// BodyMaxUTF16Units UTF-16 code units and at most PreviewMaxEvents preview
// events; truncation never splits a Unicode character. An attachment is
// required when the body cannot carry every event.
func BuildMessage(info RunInfo, c *Collector) (text string, attachmentPath string) {
	head := buildHead(info)

	// First pass: fit as many preview lines as possible while reserving space
	// for the tail notes.
	reserved := UTF16Len(head)
	if info.Outcome != OutcomeInitFailure {
		reserved += UTF16Len("\n文件事件：\n")
	}
	var lines []string
	if info.Outcome != OutcomeInitFailure {
		budget := BodyMaxUTF16Units - reserved - 256
		preview := c.Preview()
		for _, event := range preview {
			if len(lines) >= PreviewMaxEvents {
				break
			}
			line := previewLine(event, info.Threshold)
			units := UTF16Len(line)
			if units > budget {
				break
			}
			lines = append(lines, line)
			budget -= units
		}
	}

	// Second pass: build the tail for the actual preview count and drop
	// preview lines from the end until everything fits.
	for {
		tail := buildTail(info, c, len(lines))
		total := UTF16Len(head)
		if len(lines) > 0 {
			total += UTF16Len("\n文件事件：\n")
			for _, line := range lines {
				total += UTF16Len(line)
			}
		}
		total += UTF16Len(tail)
		if total <= BodyMaxUTF16Units || len(lines) == 0 {
			body := &strings.Builder{}
			body.WriteString(head)
			if len(lines) > 0 {
				body.WriteString("\n文件事件：\n")
				for _, line := range lines {
					body.WriteString(line)
				}
			}
			body.WriteString(tail)
			return body.String(), attachmentPathFor(info, c, len(lines))
		}
		lines = lines[:len(lines)-1]
	}
}

// buildHead renders the fixed heading, result, timing, source and statistics
// sections. Everything here uses the established report vocabulary.
func buildHead(info RunInfo) string {
	b := &strings.Builder{}
	fmt.Fprintf(b, "%s STRM 同步报告\n", info.InstanceName)
	fmt.Fprintf(b, "运行方式：%s\n", info.Mode)
	fmt.Fprintf(b, "结果：%s\n", outcomeLabel(info))
	fmt.Fprintf(b, "完成时间：%s\n", timestamp(info.FinishedAt))
	fmt.Fprintf(b, "同步耗时：%s\n", formatDuration(info.SyncDuration))
	if !info.StartedAt.IsZero() {
		fmt.Fprintf(b, "开始时间：%s\n", timestamp(info.StartedAt))
	}
	if info.SourceCount > 0 {
		fmt.Fprintf(b, "来源总数：%d（完成 %d / 未完成 %d）\n",
			info.SourceCount, info.CompletedSources, info.IncompleteSources)
	}
	if info.Outcome == OutcomeInitFailure || !info.StatsAvailable {
		b.WriteString("统计：不可用\n")
		return b.String()
	}
	s := info.Stats
	fmt.Fprintf(b, "统计：新增 %d · 更新 %d · 未变化跳过 %d · 扩展名过滤 %d · 实际删除 %d · 归属回收 %d · 安全保留 %d · 冲突 %d · 处理错误 %d\n",
		s.Created, s.Updated, s.Skipped, s.Filtered, s.Deleted, s.Reclaimed, s.Retained, s.Conflicts, s.Errors)
	for _, src := range info.Sources {
		if src.Completed {
			fmt.Fprintf(b, "来源 %d：完成\n", src.Index)
			continue
		}
		fmt.Fprintf(b, "来源 %d：未完成\n", src.Index)
		for i, note := range src.Notes {
			if i >= 5 {
				break
			}
			fmt.Fprintf(b, "  来源 %d：%s\n", src.Index, note)
		}
		if src.NotesMore > 0 {
			fmt.Fprintf(b, "  来源 %d：（其余 %d 条诊断省略）\n", src.Index, src.NotesMore)
		}
	}
	return b.String()
}

// previewLine renders one bounded event line for the body preview.
func previewLine(event Event, threshold int) string {
	label := kindLabels[event.Kind]
	if label == "" {
		label = "事件"
	}
	b := &strings.Builder{}
	fmt.Fprintf(b, "  %s 来源 %d %s", label, event.SourceIndex, EscapeText(event.RelativePath))
	switch event.Kind {
	case KindDeleted:
		fmt.Fprintf(b, "（删除前计数 %d/阈值 %d）", event.PriorCount, threshold)
	case KindMissingCleared:
		fmt.Fprintf(b, "（计数 %d→0/阈值 %d）", event.PriorCount, threshold)
	case KindMissingAdvanced:
		fmt.Fprintf(b, "（计数 %d→%d/阈值 %d）", event.PriorCount, event.NewCount, threshold)
	}
	b.WriteString("\n")
	return b.String()
}

// buildTail renders the omission and completeness notes. It is always short
// and always fully included in the body. shown is the number of preview lines
// actually kept in the body.
func buildTail(info RunInfo, c *Collector, shown int) string {
	b := &strings.Builder{}
	if info.Outcome == OutcomeInitFailure {
		b.WriteString("本轮同步初始化失败；未产出可报告的文件与计数事实。\n")
		return b.String()
	}
	total := c.TotalEvents()
	if total == 0 {
		b.WriteString("文件事件：本轮无新增、更新、删除或归属回收事件。\n")
	} else if total > shown {
		b.WriteString(fmt.Sprintf("文件事件：正文仅预览前 %d 条，另有 %d 条未在正文展示。\n", shown, total-shown))
	}
	if c.Degraded() {
		if c.LimitReached() {
			b.WriteString("明细不完整：附件清单达到大小上限，未包含全部事件。\n")
		}
		if c.WriteFailed() {
			b.WriteString("明细不完整：本地明细收集写入失败，清单可能缺失。\n")
		}
	}
	return b.String()
}

// attachmentPathFor decides whether the single attachment is required. It is
// required when the body preview cannot carry every event; the temporary
// listing must then exist with usable content.
func attachmentPathFor(info RunInfo, c *Collector, shown int) string {
	if info.Outcome == OutcomeInitFailure {
		return ""
	}
	total := c.TotalEvents()
	if total == 0 || total <= shown {
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
