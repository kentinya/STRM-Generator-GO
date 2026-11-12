package notify

import (
	"fmt"
	"html"
	"strings"
	"testing"
	"time"
)

// visible strips the controlled formatting tags emitted by the renderer and
// decodes its three HTML entities. It is a test reader for assertions on the
// user-visible text, not a Telegram renderer.
func visible(text string) string {
	replacer := strings.NewReplacer(
		"<b>", "", "</b>", "",
		"<code>", "", "</code>", "",
		"<blockquote expandable>", "", "</blockquote>", "",
	)
	return html.UnescapeString(replacer.Replace(text))
}

// assertEntitiesIntact checks that every ampersand in the rendered HTML belongs
// to one of the three complete entities the renderer emits. A split entity at
// a truncation boundary would fail this check.
func assertEntitiesIntact(t *testing.T, text string) {
	t.Helper()
	for i := 0; i < len(text); i++ {
		if text[i] != '&' {
			continue
		}
		end := strings.IndexByte(text[i:], ';')
		if end < 0 {
			t.Fatalf("unterminated HTML entity at byte %d:\n%s", i, text)
		}
		entity := text[i : i+end+1]
		switch entity {
		case "&amp;", "&lt;", "&gt;":
		default:
			t.Fatalf("unexpected HTML entity %q at byte %d:\n%s", entity, i, text)
		}
	}
}

func TestEscapeTextLeavesOrdinaryNamesUntouched(t *testing.T) {
	cases := []string{
		"Movies/A #100%.strm",
		"科幻/中文 目录.strm",
		"emoji-🎬.strm",
		"quote\"and'stick.strm",
	}
	for _, in := range cases {
		if got := EscapeText(in); got != in {
			t.Fatalf("EscapeText(%q) = %q, want unchanged", in, got)
		}
	}
}

func TestEscapeTextEscapesControlAndBidiCharacters(t *testing.T) {
	cases := []struct{ in, want string }{
		{"a\nb", `a\u000Ab`},
		{"a\tb", `a\u0009b`},
		{"a\x00b", `a\u0000b`},
		{"a\x7Fb", `a\u007Fb`},
		{"a\u202Eb", `a\u202Eb`},
		{"a\u200Fb", `a\u200Fb`},
		{"a\u061Cb", `a\u061Cb`},
		{"a\u2069b", `a\u2069b`},
	}
	for _, tc := range cases {
		if got := EscapeText(tc.in); got != tc.want {
			t.Fatalf("EscapeText(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestUTF16LenAndTruncation(t *testing.T) {
	if got := UTF16Len("abc"); got != 3 {
		t.Fatalf("UTF16Len(bmp) = %d", got)
	}
	if got := UTF16Len("🎬"); got != 2 {
		t.Fatalf("UTF16Len(astral) = %d", got)
	}
	s := strings.Repeat("a", 9) + "🎬" + strings.Repeat("b", 9) // 9+2+9 = 20 units
	if got := UTF16Len(s); got != 20 {
		t.Fatalf("UTF16Len(mixed) = %d", got)
	}
	if got := TruncateUTF16(s, 11); got != strings.Repeat("a", 9)+"🎬" || UTF16Len(got) != 11 {
		t.Fatalf("TruncateUTF16 split a character: %q (%d)", got, UTF16Len(got))
	}
	// A limit of 10 cannot fit the astral character: the prefix keeps the
	// complete nine ASCII characters and never splits the emoji.
	if got := TruncateUTF16(s, 10); got != strings.Repeat("a", 9) || UTF16Len(got) != 9 {
		t.Fatalf("TruncateUTF16 = %q (%d)", got, UTF16Len(got))
	}
	if got := TruncateUTF16(s, 100); got != s {
		t.Fatalf("TruncateUTF16 changed a fitting string")
	}
}

func TestShouldSendPolicy(t *testing.T) {
	success := OutcomeSuccess
	failure := OutcomeSourceFailure
	init := OutcomeInitFailure
	cases := []struct {
		policy    string
		outcome   Outcome
		anyChange bool
		want      bool
	}{
		{"all", success, false, true},
		{"all", init, false, true},
		{"error", success, true, false},
		{"error", failure, false, true},
		{"error", init, false, true},
		{"change_or_error", success, false, false},
		{"change_or_error", success, true, true},
		{"change_or_error", failure, false, true},
		{"change_or_error", init, false, true},
	}
	for _, tc := range cases {
		if got := ShouldSend(tc.policy, tc.outcome, tc.anyChange); got != tc.want {
			t.Fatalf("ShouldSend(%s, %v, %v) = %v, want %v", tc.policy, tc.outcome, tc.anyChange, got, tc.want)
		}
	}
}

func TestBuildMessageSuccessWithoutEvents(t *testing.T) {
	info := RunInfo{
		InstanceName:      "家庭媒体",
		Mode:              "serve",
		Outcome:           OutcomeSuccess,
		StartedAt:         time.Unix(1000000, 0),
		FinishedAt:        time.Unix(1000010, 0),
		SyncDuration:      10 * time.Second,
		SourceCount:       2,
		CompletedSources:  2,
		IncompleteSources: 0,
		StatsAvailable:    true,
		Stats:             Stats{Skipped: 4, Filtered: 1},
		Threshold:         3,
	}
	text, attachment := BuildMessage(info, nil)
	if attachment != "" {
		t.Fatalf("no events must not require an attachment: %q", attachment)
	}
	if !strings.Contains(text, "<b>✅ STRM 同步完成</b>") {
		t.Fatalf("bold success title missing:\n%s", text)
	}
	body := visible(text)
	for _, want := range []string{
		"🏷️ 家庭媒体 · serve",
		"➕ 新增 0 · ✏️ 更新 0",
		"🗑️ 实际删除 0 · ♻️ 归属回收 0",
		"⏭️ 未变化跳过 4 · 🔎 扩展名过滤 1",
		"🛡️ 安全保留 0 · ⚠️ 冲突 0 · ❌ 处理错误 0",
		"📂 来源 2 · 完成 2 / 未完成 0",
		"⏱️ 同步耗时 10.0 秒",
		"本轮无新增、更新、删除或归属回收事件",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("body missing %q:\n%s", want, body)
		}
	}
	// Times carry an explicit timezone.
	if !strings.Contains(body, "+") && !strings.Contains(body, "Z") {
		t.Fatalf("timestamps lack an explicit timezone: %s", body)
	}
	assertEntitiesIntact(t, text)
}

func TestBuildMessageCountsUnavailableOnInitFailure(t *testing.T) {
	info := RunInfo{InstanceName: "S", Mode: "sync", Outcome: OutcomeInitFailure, SyncDuration: time.Second}
	text, attachment := BuildMessage(info, nil)
	if attachment != "" {
		t.Fatalf("init failure must not attach a listing")
	}
	if !strings.Contains(text, "<b>❌ STRM 同步初始化失败</b>") {
		t.Fatalf("init-failure title missing:\n%s", text)
	}
	if !strings.Contains(text, "统计：不可用") {
		t.Fatalf("counts must be unavailable, not zero:\n%s", text)
	}
	if strings.Contains(text, "➕ 新增 0") {
		t.Fatalf("init failure must not render the nine counters as zero:\n%s", text)
	}
}

func TestBuildMessageConflictNeverReadsAsSuccess(t *testing.T) {
	info := RunInfo{
		InstanceName:      "S",
		Mode:              "sync",
		Outcome:           OutcomeSourceFailure,
		SourceCount:       2,
		CompletedSources:  1,
		IncompleteSources: 1,
		StatsAvailable:    true,
		Stats:             Stats{Conflicts: 1, Errors: 0},
		Sources:           []SourceStatus{{Index: 0, Notes: []string{"existing file not owned by this source: A.strm"}}},
	}
	text, _ := BuildMessage(info, nil)
	if !strings.Contains(text, "<b>⚠️ STRM 部分来源未完成</b>") {
		t.Fatalf("conflict round must not be a success:\n%s", text)
	}
	if !strings.Contains(visible(text), "existing file not owned by this source: A.strm") {
		t.Fatalf("safe diagnostic missing:\n%s", text)
	}
}

func TestBuildMessagePreviewAndAttachmentDecision(t *testing.T) {
	c, err := NewCollector("sync test", 3)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	for i := 0; i < 25; i++ {
		c.Record(Event{SourceIndex: 1, Kind: KindCreated, RelativePath: "Movies/file.strm"})
	}
	if c.TotalEvents() != 25 {
		t.Fatalf("total = %d", c.TotalEvents())
	}
	info := RunInfo{
		InstanceName: "S", Mode: "sync", Outcome: OutcomeSuccess, StatsAvailable: true,
		Stats: Stats{Created: 25}, Threshold: 3,
	}
	text, attachment := BuildMessage(info, c)
	if attachment == "" {
		t.Fatal("an attachment is required when the body cannot carry every event")
	}
	body := visible(text)
	if !strings.Contains(body, "正文预览 20 条，另有 5 条未展示。") {
		t.Fatalf("omission note missing:\n%s", body)
	}
	if !strings.Contains(body, "📎 完整清单 25 条，随附上传。") {
		t.Fatalf("attachment note missing:\n%s", body)
	}
	if !strings.Contains(text, "<blockquote expandable>") || !strings.Contains(text, "</blockquote>") {
		t.Fatalf("20 preview events must share one expandable block:\n%s", text)
	}
	shown := strings.Count(text, `<code>Movies/file.strm</code>`)
	if shown != PreviewMaxEvents {
		t.Fatalf("preview events = %d, want %d", shown, PreviewMaxEvents)
	}
}

func TestBuildMessageEscapesEventPaths(t *testing.T) {
	c, err := NewCollector("sync test", 3)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	c.Record(Event{SourceIndex: 2, Kind: KindCreated, RelativePath: "evil\nfake\u202Eevent.strm"})
	info := RunInfo{InstanceName: "S", Mode: "sync", Outcome: OutcomeSuccess, StatsAvailable: true, Stats: Stats{Created: 1}, Threshold: 3}
	text, attachment := BuildMessage(info, c)
	if !strings.Contains(text, `evil\u000Afake\u202Eevent.strm`) {
		t.Fatalf("path not escaped:\n%s", text)
	}
	if strings.Count(text, "➕ 新增 · 来源 2") != 1 {
		t.Fatalf("an escaped line break must not create a second event line:\n%s", text)
	}
	if attachment != "" {
		t.Fatalf("one fitting event must not require an attachment")
	}
}

func TestCollectorStreamsWithSizeCapAndTrailer(t *testing.T) {
	c, err := NewCollector("sync cap", 3)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	long := strings.Repeat("长", 4096) // ~12 KiB per event in UTF-8
	for i := 0; i < 4096; i++ {
		c.Record(Event{SourceIndex: 1, Kind: KindMissingAdvanced, RelativePath: long, PriorCount: 1, NewCount: 2, Reason: ReasonSourceTarget})
	}
	if err := c.Finalize(); err != nil {
		t.Fatal(err)
	}
	if c.TotalEvents() != 4096 {
		t.Fatalf("total = %d", c.TotalEvents())
	}
	if !c.LimitReached() || !c.Degraded() {
		t.Fatalf("limit not detected: limitHit=%v broken=%v", c.limitHit, c.broken)
	}
	path, ok := c.AttachmentPath()
	if !ok {
		t.Fatal("a partial listing is still a usable attachment")
	}
	data, err := readFile(t, path)
	if err != nil {
		t.Fatal(err)
	}
	if int64(len(data)) > AttachmentMaxBytes {
		t.Fatalf("attachment exceeds the size cap: %d", len(data))
	}
	content := string(data)
	if !strings.Contains(content, "清单不完整") {
		t.Fatal("trailer must mark the listing as incomplete")
	}
	if !strings.Contains(content, "事件总数: 4096") {
		t.Fatal("trailer must keep the total count")
	}
	lines := strings.Count(content, "来源 1 缺失计数")
	if lines == 0 || lines == 4096 {
		t.Fatalf("partially written listing has %d event lines", lines)
	}
}

func TestCollectorWriteFailureDegradesWithoutAttachment(t *testing.T) {
	c, err := NewCollector("sync broken", 3)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	// Simulate a failing backing file without touching production behavior.
	if err := c.file.Close(); err != nil {
		t.Fatal(err)
	}
	c.Record(Event{SourceIndex: 1, Kind: KindCreated, RelativePath: "A.strm"})
	if err := c.Finalize(); err != nil {
		t.Fatal(err)
	}
	if !c.WriteFailed() {
		t.Fatal("write failure must be recorded")
	}
	if _, ok := c.AttachmentPath(); ok {
		t.Fatal("a broken listing must not be uploaded")
	}
	info := RunInfo{InstanceName: "S", Mode: "sync", Outcome: OutcomeSuccess, StatsAvailable: true, Stats: Stats{Created: 1}, Threshold: 3}
	text, attachment := BuildMessage(info, c)
	if attachment != "" {
		t.Fatalf("a broken listing must not be attached")
	}
	if !strings.Contains(text, "明细不完整") {
		t.Fatalf("body must mark the degraded detail:\n%s", text)
	}
}

func TestCollectorEventLineVocabulary(t *testing.T) {
	c, err := NewCollector("sync lines", 3)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	c.Record(Event{SourceIndex: 1, Kind: KindDeleted, RelativePath: "M/A.strm", PriorCount: 3, Reason: ReasonSourceTarget})
	c.Record(Event{SourceIndex: 1, Kind: KindReclaimed, RelativePath: "M/B.strm", Reason: ReasonExcludedExtension})
	c.Record(Event{SourceIndex: 2, Kind: KindMissingCleared, RelativePath: "N/C.strm", PriorCount: 2})
	c.Record(Event{SourceIndex: 2, Kind: KindMissingAdvanced, RelativePath: "N/D.strm", PriorCount: 1, NewCount: 2, Reason: ReasonExcludedExtension})
	if err := c.Finalize(); err != nil {
		t.Fatal(err)
	}
	path, ok := c.AttachmentPath()
	if !ok {
		t.Fatal("listing missing")
	}
	data, err := readFile(t, path)
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	for _, want := range []string{
		"1 来源 1 实际删除 M/A.strm 删除前计数 3/阈值 3 已实际删除 原因 源目标未读到",
		"2 来源 1 归属回收 M/B.strm 本地目标此前已不存在，归属已移除 原因 扩展名排除",
		"3 来源 2 取消待删除 N/C.strm 计数 2→0/阈值 3 已确认恢复条件，文件保留",
		"4 来源 2 缺失计数 N/D.strm 计数 1→2/阈值 3 原因 扩展名排除；待删除计数推进，文件仍保留",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("listing missing %q:\n%s", want, content)
		}
	}
}

// B review B-07: every preview line carries its qualification reason and its
// stage in the cleanup pipeline, including the threshold note that waiting for
// the re-check is not the deletion itself.
func TestPreviewLinesCarryReasonAndStage(t *testing.T) {
	cases := []struct {
		event Event
		want  string
	}{
		{
			Event{SourceIndex: 1, Kind: KindMissingAdvanced, RelativePath: "A.strm", PriorCount: 0, NewCount: 1, Reason: ReasonSourceTarget},
			"⏳ 待删除计数 · 来源 1 · A.strm（计数 0→1/阈值 3，原因 源目标未读到，首次进入待删除，文件仍保留）",
		},
		{
			Event{SourceIndex: 1, Kind: KindMissingAdvanced, RelativePath: "A.strm", PriorCount: 1, NewCount: 2, Reason: ReasonExcludedExtension},
			"（计数 1→2/阈值 3，原因 扩展名排除，待删除计数推进，文件仍保留）",
		},
		{
			Event{SourceIndex: 1, Kind: KindMissingAdvanced, RelativePath: "A.strm", PriorCount: 2, NewCount: 3, Reason: ReasonSourceTarget},
			"（计数 2→3/阈值 3，原因 源目标未读到，达到阈值，等待后续清理重核，不等于已删除）",
		},
		{
			Event{SourceIndex: 1, Kind: KindMissingCleared, RelativePath: "A.strm", PriorCount: 2},
			"↩️ 恢复保留 · 来源 1 · A.strm（计数 2→0/阈值 3，已确认恢复条件，文件保留）",
		},
		{
			Event{SourceIndex: 1, Kind: KindDeleted, RelativePath: "A.strm", PriorCount: 3, Reason: ReasonExcludedExtension},
			"🗑️ 实际删除 · 来源 1 · A.strm（删除前计数 3/阈值 3，已实际删除，原因 扩展名排除）",
		},
		{
			Event{SourceIndex: 1, Kind: KindReclaimed, RelativePath: "A.strm"},
			"♻️ 归属回收 · 来源 1 · A.strm（本地目标此前已不存在，归属已移除）",
		},
	}
	for _, tc := range cases {
		got := previewLine(tc.event, 3)
		if !strings.Contains(got, "<code>A.strm</code>") {
			t.Fatalf("preview path must stay complete in code markup: %q", got)
		}
		if !strings.Contains(visible(got), tc.want) {
			t.Fatalf("preview line = %q, want %q", got, tc.want)
		}
	}
}

// B review B-04: the whole body shares one budget. Many failed sources must
// never push the body past the limit, and the omitted source details must go
// into the same single attachment.
func TestBuildMessageBoundsWholeBodyWithSourceOverflow(t *testing.T) {
	var sources []SourceStatus
	for i := 0; i < 60; i++ {
		note := fmt.Sprintf("source root %d not readable as a directory; no round committed", i)
		sources = append(sources, SourceStatus{
			Index:       i,
			Notes:       []string{note},
			DetailNotes: []string{note},
		})
	}
	info := RunInfo{
		InstanceName: "S", Mode: "sync", Outcome: OutcomeSourceFailure,
		SourceCount: 60, IncompleteSources: 60, StatsAvailable: true,
		Stats: Stats{Errors: 60}, Threshold: 3, Sources: sources,
	}
	c, err := NewCollector("sync overflow", 3)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	text, attachment := BuildMessage(info, c)
	if UTF16Len(text) > BodyMaxUTF16Units {
		t.Fatalf("body = %d UTF-16 units, over the limit", UTF16Len(text))
	}
	if attachment == "" {
		t.Fatal("source overflow must require the single attachment")
	}
	data, err := readFile(t, attachment)
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	// Every sampled source detail omitted from the body reaches the
	// attachment with its full value.
	bodyShown := strings.Count(text, "not readable as a directory")
	count := strings.Count(content, "not readable as a directory")
	if count != 60-bodyShown || count == 0 {
		t.Fatalf("attachment carries %d source details, want %d:\n%s", count, 60-bodyShown, content)
	}
	if !strings.Contains(text, "来源明细：另有") {
		t.Fatalf("body must note the omitted source details:\n%s", text)
	}
	if err := c.Finalize(); err != nil {
		t.Fatal(err)
	}
}

// B review B-04: with zero file events, source overflow alone must still
// produce the single attachment.
func TestBuildMessageSourceOverflowWithoutEventsRequiresAttachment(t *testing.T) {
	var sources []SourceStatus
	for i := 0; i < 40; i++ {
		note := fmt.Sprintf("ownership record %d lacks an exact remote mapping for the configured source root and is preserved; %s", i, strings.Repeat("x", 60))
		sources = append(sources, SourceStatus{
			Index:       i,
			Completed:   true,
			Retained:    1,
			Notes:       []string{note},
			DetailNotes: []string{note},
		})
	}
	info := RunInfo{
		InstanceName: "S", Mode: "serve", Outcome: OutcomeSuccess,
		SourceCount: 40, CompletedSources: 40, StatsAvailable: true,
		Stats: Stats{Retained: 40}, Threshold: 3, Sources: sources,
	}
	c, err := NewCollector("serve retained", 3)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	text, attachment := BuildMessage(info, c)
	if UTF16Len(text) > BodyMaxUTF16Units {
		t.Fatalf("body = %d UTF-16 units", UTF16Len(text))
	}
	if attachment == "" {
		t.Fatal("source overflow with zero events must still attach the listing")
	}
}

// B review B-05: source diagnostics and the instance name are dynamic display
// content and must be escaped like file paths.
func TestBuildMessageEscapesNotesAndInstanceName(t *testing.T) {
	info := RunInfo{
		InstanceName: " evil\u202Ename", Mode: "sync", Outcome: OutcomeSourceFailure,
		SourceCount: 1, IncompleteSources: 1, StatsAvailable: true,
		Stats:   Stats{Conflicts: 1},
		Sources: []SourceStatus{{Index: 0, Notes: []string{"existing file not owned by this source: evil\u202E.strm"}}},
	}
	text, _ := BuildMessage(info, nil)
	if strings.Contains(text, "\u202E") {
		t.Fatalf("a bidi control character must be escaped:\n%s", text)
	}
	if !strings.Contains(text, `evil\u202Ename`) || !strings.Contains(text, `evil\u202E.strm`) {
		t.Fatalf("escaped values missing:\n%s", text)
	}
}

// B review B-06: without a collector the summary still sends; the detail and
// the missing-count progress are explicitly unknown, never zero.
func TestBuildMessageWithoutCollectorMarksDetailUnavailable(t *testing.T) {
	info := RunInfo{
		InstanceName: "S", Mode: "sync", Outcome: OutcomeSuccess,
		SourceCount: 1, CompletedSources: 1, StatsAvailable: true,
		Stats: Stats{Skipped: 1}, Threshold: 3, DetailUnavailable: true,
	}
	text, attachment := BuildMessage(info, nil)
	if attachment != "" {
		t.Fatal("no listing exists when collection was unavailable")
	}
	if !strings.Contains(text, "明细不完整：本轮明细收集不可用，文件事件与缺失计数进度未知。") {
		t.Fatalf("detail-unavailable note missing:\n%s", text)
	}
	if strings.Contains(text, "本轮无新增、更新、删除或归属回收事件") {
		t.Fatalf("an unknown detail must never read as zero events:\n%s", text)
	}
}

// B review: an initialization failure keeps the configured source count while
// the per-source completion stays unavailable.
func TestBuildMessageInitFailureKeepsConfiguredSourceCount(t *testing.T) {
	info := RunInfo{
		InstanceName: "S", Mode: "sync", Outcome: OutcomeInitFailure,
		SourceCount: 5, SyncDuration: time.Second,
	}
	text, _ := BuildMessage(info, nil)
	if !strings.Contains(text, "📂 来源 5 · 完成/未完成计数不可用") {
		t.Fatalf("configured source count lost:\n%s", text)
	}
	if !strings.Contains(text, "统计：不可用") {
		t.Fatalf("counts must stay unavailable:\n%s", text)
	}
}

// B review B-01: provisional advance records are removed when the commit
// fails and kept when it succeeds.
func TestCollectorProvisionalRecordsDiscardOnFailedCommit(t *testing.T) {
	c, err := NewCollector("sync provisional", 3)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	confirmed := Event{SourceIndex: 1, Kind: KindCreated, RelativePath: "A.strm"}
	c.Record(confirmed)

	c.RecordProvisional(Event{SourceIndex: 1, Kind: KindMissingAdvanced, RelativePath: "B.strm", PriorCount: 0, NewCount: 1, Reason: ReasonSourceTarget})
	if c.TotalEvents() != 2 || len(c.Preview()) != 2 {
		t.Fatalf("provisional records must be visible before the decision: %+v", c)
	}
	c.DiscardProvisional()
	if c.TotalEvents() != 1 || len(c.Preview()) != 1 {
		t.Fatalf("discarded records must be fully removed: total=%d preview=%d", c.TotalEvents(), len(c.Preview()))
	}
	if c.Degraded() {
		t.Fatal("a clean discard must not degrade the listing")
	}

	// With nothing but provisional records, a discard must also revert the
	// change flag: an uncommitted candidate never triggers change_or_error.
	empty, err := NewCollector("sync provisional only", 3)
	if err != nil {
		t.Fatal(err)
	}
	defer empty.Close()
	empty.RecordProvisional(Event{SourceIndex: 1, Kind: KindMissingAdvanced, RelativePath: "C.strm", PriorCount: 0, NewCount: 1})
	if !empty.HasChange() {
		t.Fatal("a provisional record is observable before the decision")
	}
	empty.DiscardProvisional()
	if empty.HasChange() || empty.TotalEvents() != 0 {
		t.Fatal("a discarded advance must not count as a change")
	}

	// A second attempt that commits keeps the record permanently.
	c.RecordProvisional(Event{SourceIndex: 1, Kind: KindMissingAdvanced, RelativePath: "B.strm", PriorCount: 0, NewCount: 1, Reason: ReasonSourceTarget})
	c.CommitProvisional()
	if err := c.Finalize(); err != nil {
		t.Fatal(err)
	}
	path, ok := c.AttachmentPath()
	if !ok {
		t.Fatal("listing missing")
	}
	data, err := readFile(t, path)
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	if strings.Count(content, "B.strm") != 1 {
		t.Fatalf("listing must carry exactly the committed record:\n%s", content)
	}
	if !strings.Contains(content, "事件总数: 2") {
		t.Fatalf("trailer totals:\n%s", content)
	}
}

// A provisional record that hits the size cap is removed again by a discard
// without falsely claiming an incomplete listing.
func TestCollectorProvisionalDiscardRevertsLimit(t *testing.T) {
	c, err := NewCollector("sync provisional cap", 3)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	long := strings.Repeat("长", 4096)
	for !c.LimitReached() {
		c.RecordProvisional(Event{SourceIndex: 1, Kind: KindMissingAdvanced, RelativePath: long, PriorCount: 0, NewCount: 1})
	}
	c.DiscardProvisional()
	if c.LimitReached() || c.Degraded() {
		t.Fatal("the reverted records must not leave a degradation behind")
	}
}

// B re-review B-R1: even when the listing file can no longer be rewound, the
// discard must withdraw the uncommitted logical facts so a rolled-back
// candidate never counts as progress or triggers change_or_error. The
// write-failure marker stays so the unusable listing is never uploaded.
func TestCollectorDiscardAfterWriteFailureWithdrawsFacts(t *testing.T) {
	c, err := NewCollector("sync discard broken", 3)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	committed := Event{SourceIndex: 1, Kind: KindCreated, RelativePath: "A.strm"}
	c.Record(committed)
	if c.TotalEvents() != 1 || !c.HasChange() {
		t.Fatalf("committed fact: total=%d change=%v", c.TotalEvents(), c.HasChange())
	}
	// The backing file fails from here on.
	if err := c.file.Close(); err != nil {
		t.Fatal(err)
	}
	c.RecordProvisional(Event{SourceIndex: 1, Kind: KindMissingAdvanced, RelativePath: "B.strm", PriorCount: 1, NewCount: 2, Reason: ReasonSourceTarget})
	if !c.WriteFailed() {
		t.Fatal("the failing write must be recorded")
	}
	c.DiscardProvisional()
	if c.TotalEvents() != 1 || len(c.Preview()) != 1 || c.Preview()[0] != committed {
		t.Fatalf("uncommitted candidate stayed behind: total=%d preview=%+v", c.TotalEvents(), c.Preview())
	}
	if !c.HasChange() {
		t.Fatal("the already committed fact must keep the change flag")
	}
	if !c.WriteFailed() || c.LimitReached() {
		t.Fatalf("degradation markers: broken=%v limit=%v", c.broken, c.limitHit)
	}
	if _, ok := c.AttachmentPath(); ok {
		t.Fatal("a broken listing must never be uploaded")
	}

	// A later committed provisional record stays visible in memory and the
	// trailer totals remain truthful about what the listing carries.
	c.RecordProvisional(Event{SourceIndex: 1, Kind: KindMissingAdvanced, RelativePath: "B.strm", PriorCount: 1, NewCount: 2})
	c.CommitProvisional()
	if err := c.Finalize(); err != nil {
		t.Fatal(err)
	}
	if c.TotalEvents() != 2 || len(c.Preview()) != 2 {
		t.Fatalf("committed record after failure: total=%d preview=%d", c.TotalEvents(), len(c.Preview()))
	}
	info := RunInfo{
		InstanceName: "S", Mode: "sync", Outcome: OutcomeSuccess,
		SourceCount: 1, CompletedSources: 1, StatsAvailable: true, Threshold: 3,
	}
	text, attachment := BuildMessage(info, c)
	if attachment != "" {
		t.Fatalf("a broken listing must not be attached")
	}
	if !strings.Contains(text, "明细不完整：本地明细收集写入失败，清单可能缺失。") {
		t.Fatalf("write-failure note missing:\n%s", text)
	}
}

// B re-review B-R2: a long legal instance name is budgeted in the display
// layer — the body never exceeds the limit, keeps its statistics and marks
// the omission instead of rejecting the configuration value.
func TestBuildMessageLongInstanceNameStaysBounded(t *testing.T) {
	info := RunInfo{
		InstanceName: strings.Repeat("家", 5000), Mode: "sync", Outcome: OutcomeSuccess,
		SourceCount: 1, CompletedSources: 1, StatsAvailable: true,
		Stats: Stats{Skipped: 1}, Threshold: 3,
	}
	text, _ := BuildMessage(info, nil)
	if n := UTF16Len(text); n > BodyMaxUTF16Units {
		t.Fatalf("body = %d UTF-16 units, over the limit", n)
	}
	if !strings.Contains(text, "…") {
		t.Fatalf("the truncated instance name must carry an omission marker:\n%s", text)
	}
	if !strings.Contains(text, "📂 来源 1 · 完成 1 / 未完成 0") {
		t.Fatalf("statistics lost to the long instance name:\n%s", text)
	}
	assertEntitiesIntact(t, text)
	if strings.Contains(text, strings.Repeat("家", 4000)) {
		t.Fatal("the full instance name must not survive in the body")
	}

	// Escaped bidirectional control characters inflate the display value far
	// beyond the raw length; they pass the single-line validation, so the
	// budget applies after escaping and never emits a raw control character.
	expanded := RunInfo{
		InstanceName: strings.Repeat("\u202e", 800), Mode: "serve", Outcome: OutcomeSourceFailure,
		SourceCount: 1, IncompleteSources: 1, StatsAvailable: true,
		Stats: Stats{Errors: 1}, Threshold: 3,
	}
	text2, _ := BuildMessage(expanded, nil)
	if n := UTF16Len(text2); n > BodyMaxUTF16Units {
		t.Fatalf("escaped-expansion body = %d UTF-16 units, over the limit", n)
	}
	if strings.Contains(text2, "\u202e") {
		t.Fatalf("a raw bidirectional control character leaked into the body:\n%q", text2)
	}
	if !strings.Contains(text2, `\u202E`) {
		t.Fatalf("the escaped instance value missing:\n%s", text2)
	}
}

// B re-review B-R3: when the overflow source details cannot be written, the
// body must report the incompleteness and never promise an unusable
// attachment as a complete listing.
func TestBuildMessageSourceDetailWriteFailureMarksBodyIncomplete(t *testing.T) {
	var sources []SourceStatus
	for i := 0; i < 60; i++ {
		note := fmt.Sprintf("source root %d not readable as a directory; no round committed", i)
		sources = append(sources, SourceStatus{
			Index:       i,
			Notes:       []string{note},
			DetailNotes: []string{note},
		})
	}
	info := RunInfo{
		InstanceName: "S", Mode: "sync", Outcome: OutcomeSourceFailure,
		SourceCount: 60, IncompleteSources: 60, StatsAvailable: true,
		Stats: Stats{Errors: 60}, Threshold: 3, Sources: sources,
	}
	c, err := NewCollector("sync overflow broken", 3)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if err := c.file.Close(); err != nil {
		t.Fatal(err)
	}
	text, attachment := BuildMessage(info, c)
	if attachment != "" {
		t.Fatal("a broken listing must not be attached")
	}
	if strings.Contains(text, "完整内容见附件清单") {
		t.Fatalf("the body must not promise a complete listing that cannot exist:\n%s", text)
	}
	if !strings.Contains(text, "明细不完整") {
		t.Fatalf("incompleteness note missing:\n%s", text)
	}
	if n := UTF16Len(text); n > BodyMaxUTF16Units {
		t.Fatalf("body = %d UTF-16 units, over the limit", n)
	}
}

// B re-review B-R3: a final trailer write failure after usable event appends
// must still flip the body to the incompleteness wording and drop the
// attachment; the round facts themselves stay untouched.
func TestBuildMessageTrailerWriteFailureMarksBodyIncomplete(t *testing.T) {
	c, err := NewCollector("sync trailer broken", 3)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	c.Record(Event{SourceIndex: 1, Kind: KindCreated, RelativePath: "Movies/A.strm"})
	if c.WriteFailed() {
		t.Fatal("the event write itself must succeed first")
	}
	if err := c.file.Close(); err != nil {
		t.Fatal(err)
	}
	info := RunInfo{
		InstanceName: "S", Mode: "sync", Outcome: OutcomeSuccess,
		SourceCount: 1, CompletedSources: 1, StatsAvailable: true,
		Stats: Stats{Created: 1}, Threshold: 3,
	}
	text, attachment := BuildMessage(info, c)
	if attachment != "" {
		t.Fatal("a listing without its trailer must not be attached")
	}
	if !strings.Contains(text, "明细不完整：本地明细收集写入失败，清单可能缺失。") {
		t.Fatalf("trailer-failure note missing:\n%s", text)
	}
	if strings.Contains(text, "完整内容见附件清单") {
		t.Fatalf("no complete-listing promise may survive:\n%s", text)
	}
}

// FIX-TG-01: the four result headings keep their fixed icons and wording; an
// initialization failure never renders the nine counters as zero.
func TestBuildMessageOutcomeTitles(t *testing.T) {
	cases := []struct {
		name string
		info RunInfo
		want string
	}{
		{
			name: "success",
			info: RunInfo{InstanceName: "S", Mode: "sync", Outcome: OutcomeSuccess,
				SourceCount: 1, CompletedSources: 1, StatsAvailable: true},
			want: "<b>✅ STRM 同步完成</b>",
		},
		{
			name: "partial",
			info: RunInfo{InstanceName: "S", Mode: "sync", Outcome: OutcomeSourceFailure,
				SourceCount: 2, CompletedSources: 1, IncompleteSources: 1, StatsAvailable: true,
				Stats: Stats{Errors: 1}},
			want: "<b>⚠️ STRM 部分来源未完成</b>",
		},
		{
			name: "all incomplete",
			info: RunInfo{InstanceName: "S", Mode: "serve", Outcome: OutcomeSourceFailure,
				SourceCount: 2, IncompleteSources: 2, StatsAvailable: true},
			want: "<b>❌ STRM 同步失败（全部来源未完成）</b>",
		},
		{
			name: "initialization failure",
			info: RunInfo{InstanceName: "S", Mode: "sync", Outcome: OutcomeInitFailure,
				SourceCount: 3, SyncDuration: time.Second},
			want: "<b>❌ STRM 同步初始化失败</b>",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			text, attachment := BuildMessage(tc.info, nil)
			if attachment != "" {
				t.Fatalf("this case must not attach a listing")
			}
			if !strings.Contains(text, tc.want) {
				t.Fatalf("title missing %q:\n%s", tc.want, text)
			}
			assertOnlyAllowedTags(t, text)
			assertEntitiesIntact(t, text)
		})
	}
}

// FIX-TG-01: five preview events stay directly visible; six or more share one
// expandable block while the heading stays outside. An all-fitting round does
// not require an attachment merely to show the block.
func TestBuildMessageEventCollapseBoundary(t *testing.T) {
	build := func(count int) (string, string) {
		c, err := NewCollector("sync boundary", 3)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(c.Close)
		for i := 0; i < count; i++ {
			c.Record(Event{SourceIndex: 0, Kind: KindCreated, RelativePath: fmt.Sprintf("F%02d.strm", i)})
		}
		info := RunInfo{InstanceName: "S", Mode: "sync", Outcome: OutcomeSuccess,
			SourceCount: 1, CompletedSources: 1, StatsAvailable: true,
			Stats: Stats{Created: count}, Threshold: 3}
		return BuildMessage(info, c)
	}

	five, attachment := build(5)
	if attachment != "" {
		t.Fatalf("five fitting events must not require an attachment")
	}
	if strings.Contains(five, "<blockquote expandable>") {
		t.Fatalf("five events must stay directly visible:\n%s", five)
	}
	if strings.Count(five, "<code>F") != 5 {
		t.Fatalf("five event paths missing:\n%s", five)
	}

	six, attachment := build(6)
	if attachment != "" {
		t.Fatalf("six fitting events must not force an attachment")
	}
	if strings.Count(six, "<blockquote expandable>") != 1 || strings.Count(six, "</blockquote>") != 1 {
		t.Fatalf("six events must share one expandable block:\n%s", six)
	}
	title := strings.Index(six, "<b>📋 文件事件</b>")
	open := strings.Index(six, "<blockquote expandable>")
	close := strings.Index(six, "</blockquote>")
	if title < 0 || open < 0 || close < 0 || !(title < open && open < close) {
		t.Fatalf("event heading must stay outside the block:\n%s", six)
	}
	if strings.Count(six, "<code>F") != 6 {
		t.Fatalf("six event paths missing:\n%s", six)
	}
	assertOnlyAllowedTags(t, six)
}

// FIX-TG-01: dynamic instance names, paths and diagnostics are escaped before
// HTML escaping. A literal entity in a file name stays literal and a crafted
// tag never becomes markup.
func TestBuildMessageDynamicValuesCannotInjectHTML(t *testing.T) {
	c, err := NewCollector("sync html", 3)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	c.Record(Event{SourceIndex: 0, Kind: KindCreated, RelativePath: "&lt;b&gt;.strm"})
	info := RunInfo{
		InstanceName: "库 & <b>", Mode: "sync", Outcome: OutcomeSourceFailure,
		SourceCount: 1, CompletedSources: 1, StatsAvailable: true,
		Stats: Stats{Created: 1}, Threshold: 3,
		Sources: []SourceStatus{{
			Index:     0,
			Completed: true,
			Notes:     []string{"bad <script>&\"quote\""},
		}},
	}
	text, _ := BuildMessage(info, c)
	assertOnlyAllowedTags(t, text)
	assertEntitiesIntact(t, text)
	for _, raw := range []string{
		`🏷️ 库 &amp; &lt;b&gt; · sync`,
		`<code>&amp;lt;b&amp;gt;.strm</code>`,
		`bad &lt;script&gt;&amp;"quote"`,
	} {
		if !strings.Contains(text, raw) {
			t.Fatalf("escaped value %q missing:\n%s", raw, text)
		}
	}
	body := visible(text)
	if !strings.Contains(body, "库 & <b> · sync") || !strings.Contains(body, "&lt;b&gt;.strm") {
		t.Fatalf("literal values lost after parsing:\n%s", body)
	}
	if strings.Contains(text, "<script>") || strings.Contains(text, "<b> &") {
		t.Fatalf("crafted markup leaked into the body:\n%s", text)
	}
}

// FIX-TG-01: a path too long for the body moves wholly to the attachment and
// is never shown as a shortened identity.
func TestBuildMessageLongPathMovesWholeToAttachment(t *testing.T) {
	c, err := NewCollector("sync long path", 3)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	longPath := strings.Repeat("目录/文件", 800) + ".strm"
	c.Record(Event{SourceIndex: 2, Kind: KindCreated, RelativePath: longPath})
	info := RunInfo{InstanceName: "S", Mode: "sync", Outcome: OutcomeSuccess,
		SourceCount: 1, CompletedSources: 1, StatsAvailable: true,
		Stats: Stats{Created: 1}, Threshold: 3}
	text, attachment := BuildMessage(info, c)
	if UTF16Len(text) > BodyMaxUTF16Units {
		t.Fatalf("body = %d UTF-16 units, over the limit", UTF16Len(text))
	}
	if attachment == "" {
		t.Fatal("the overflow event must use the attachment")
	}
	if strings.Contains(text, longPath[:30]) {
		t.Fatalf("a shortened path leaked into the body:\n%s", text)
	}
	if !strings.Contains(visible(text), "正文预览 0 条，另有 1 条未展示。") {
		t.Fatalf("preview omission note missing:\n%s", visible(text))
	}
	data, err := readFile(t, attachment)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), longPath) {
		t.Fatal("the complete path is missing from the attachment")
	}
}

// FIX-TG-01: source diagnostics stay before the event preview and outside the
// expandable block; a degradation warning is also outside the block.
func TestBuildMessageDiagnosticsAndWarningsStayOutsideBlock(t *testing.T) {
	c, err := NewCollector("sync outside", 3)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	for i := 0; i < 6; i++ {
		c.Record(Event{SourceIndex: 0, Kind: KindCreated, RelativePath: fmt.Sprintf("F%d.strm", i)})
	}
	info := RunInfo{
		InstanceName: "S", Mode: "sync", Outcome: OutcomeSourceFailure,
		SourceCount: 1, IncompleteSources: 1, StatsAvailable: true,
		Stats: Stats{Errors: 1}, Threshold: 3,
		Sources: []SourceStatus{{
			Index: 0,
			Notes: []string{"source root /private/path is not readable as a directory"},
		}},
	}
	text, _ := BuildMessage(info, c)
	note := strings.Index(text, "source root")
	title := strings.Index(text, "<b>📋 文件事件</b>")
	open := strings.Index(text, "<blockquote expandable>")
	close := strings.Index(text, "</blockquote>")
	if note < 0 || title < 0 || open < 0 || close < 0 || !(note < title && title < open && open < close) {
		t.Fatalf("diagnostics must precede the event block:\n%s", text)
	}

	// A local write failure leaves the six preview events visible but the
	// listing unusable; the warning must stay outside the block. A fresh
	// collector is failed before the first BuildMessage so Finalize observes
	// the broken state.
	broken, err := NewCollector("sync outside broken", 3)
	if err != nil {
		t.Fatal(err)
	}
	defer broken.Close()
	for i := 0; i < 6; i++ {
		broken.Record(Event{SourceIndex: 0, Kind: KindCreated, RelativePath: fmt.Sprintf("F%d.strm", i)})
	}
	if err := broken.file.Close(); err != nil {
		t.Fatal(err)
	}
	text, attachment := BuildMessage(info, broken)
	if attachment != "" {
		t.Fatal("a broken listing must not be attached")
	}
	open = strings.Index(text, "<blockquote expandable>")
	close = strings.Index(text, "</blockquote>")
	warning := strings.Index(text, "明细不完整")
	if warning < 0 || open < 0 || close < 0 || warning < close {
		t.Fatalf("degradation warning must stay outside the block:\n%s", text)
	}
}

// FIX-TG-01: a legal instance name made of escape-expanding characters is
// shortened by whole entities, never by cutting an entity or a Unicode
// character, and the complete HTML body stays within the budget.
func TestBuildMessageEntityHeavyInstanceStaysWithinBudget(t *testing.T) {
	info := RunInfo{
		InstanceName: strings.Repeat("&<>🎬", 1200), Mode: "serve", Outcome: OutcomeSuccess,
		SourceCount: 1, CompletedSources: 1, StatsAvailable: true,
		Stats: Stats{Created: 1}, Threshold: 3,
	}
	text, _ := BuildMessage(info, nil)
	if n := UTF16Len(text); n > BodyMaxUTF16Units {
		t.Fatalf("body = %d UTF-16 units, over the limit", n)
	}
	if !strings.Contains(text, "…") {
		t.Fatal("the shortened instance name must carry an omission marker")
	}
	if !strings.Contains(text, "➕ 新增 1") || !strings.Contains(text, "📂 来源 1 · 完成 1 / 未完成 0") {
		t.Fatalf("statistics lost:\n%s", text)
	}
	assertOnlyAllowedTags(t, text)
	assertEntitiesIntact(t, text)
}

// assertOnlyAllowedTags rejects any raw markup other than the fixed tags the
// renderer is allowed to emit.
func assertOnlyAllowedTags(t *testing.T, text string) {
	t.Helper()
	allowed := []string{
		"<b>", "</b>", "<code>", "</code>",
		"<blockquote expandable>", "</blockquote>",
	}
	for i := 0; i < len(text); i++ {
		if text[i] != '<' {
			continue
		}
		matched := false
		for _, tag := range allowed {
			if strings.HasPrefix(text[i:], tag) {
				matched = true
				i += len(tag) - 1
				break
			}
		}
		if !matched {
			t.Fatalf("unexpected raw markup at byte %d:\n%s", i, text)
		}
	}
}

// FIX-TG-01: one synthetic 25-event round is logged as the checkable raw HTML
// and visible-text sample for review. It also re-checks the 20-event preview,
// single expandable block and truthful omission/attachment notes.
func TestBuildMessageSyntheticSample(t *testing.T) {
	c, err := NewCollector("sync 2026-11-12T13:33:44Z", 3)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	for i := 1; i <= 25; i++ {
		c.Record(Event{SourceIndex: 0, Kind: KindCreated, RelativePath: fmt.Sprintf("样例 %02d.strm", i)})
	}
	info := RunInfo{
		InstanceName: "Telegram 隔离验证 attachment", Mode: "sync", Outcome: OutcomeSuccess,
		StartedAt:    time.Date(2026, 11, 12, 13, 33, 44, 0, time.UTC),
		FinishedAt:   time.Date(2026, 11, 12, 13, 33, 45, 0, time.UTC),
		SyncDuration: 403 * time.Millisecond, SourceCount: 1, CompletedSources: 1,
		StatsAvailable: true, Stats: Stats{Created: 25}, Threshold: 3,
	}
	text, attachment := BuildMessage(info, c)
	if attachment == "" {
		t.Fatal("the 25-event sample requires the single attachment")
	}
	body := visible(text)
	for _, want := range []string{
		"✅ STRM 同步完成",
		"🏷️ Telegram 隔离验证 attachment · sync",
		"➕ 新增 25 · ✏️ 更新 0",
		"📂 来源 1 · 完成 1 / 未完成 0",
		"⏱️ 同步耗时 403 毫秒",
		"正文预览 20 条，另有 5 条未展示。",
		"📎 完整清单 25 条，随附上传。",
		"🕒 开始 2026-11-12T13:33:44Z",
		"🕒 完成 2026-11-12T13:33:45Z",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("sample body missing %q:\n%s", want, body)
		}
	}
	assertOnlyAllowedTags(t, text)
	assertEntitiesIntact(t, text)
	t.Logf("RAW HTML:\n%s", text)
	t.Logf("VISIBLE TEXT:\n%s", body)
}
