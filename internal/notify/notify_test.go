package notify

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

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
	for _, want := range []string{
		"家庭媒体 STRM 同步报告",
		"运行方式：serve",
		"结果：成功",
		"同步耗时：10.0 秒",
		"来源总数：2（完成 2 / 未完成 0）",
		"本轮无新增、更新、删除或归属回收事件",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("body missing %q:\n%s", want, text)
		}
	}
	// Times carry an explicit timezone.
	if !strings.Contains(text, "+") && !strings.Contains(text, "Z") {
		t.Fatalf("timestamps lack an explicit timezone: %s", text)
	}
}

func TestBuildMessageCountsUnavailableOnInitFailure(t *testing.T) {
	info := RunInfo{InstanceName: "S", Mode: "sync", Outcome: OutcomeInitFailure, SyncDuration: time.Second}
	text, attachment := BuildMessage(info, nil)
	if attachment != "" {
		t.Fatalf("init failure must not attach a listing")
	}
	if !strings.Contains(text, "统计：不可用") {
		t.Fatalf("counts must be unavailable, not zero:\n%s", text)
	}
	if !strings.Contains(text, "同步初始化失败") {
		t.Fatalf("body:\n%s", text)
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
	if !strings.Contains(text, "部分来源未完成") {
		t.Fatalf("conflict round must not be a success:\n%s", text)
	}
	if !strings.Contains(text, "existing file not owned by this source: A.strm") {
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
	if !strings.Contains(text, "另有 5 条未在正文展示") {
		t.Fatalf("omission note missing:\n%s", text)
	}
	shown := strings.Count(text, "新增 来源 1 Movies/file.strm")
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
	if strings.Count(text, "新增 来源") != 1 {
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
			"缺失计数 来源 1 A.strm（计数 0→1/阈值 3，原因 源目标未读到，首次进入待删除，文件仍保留）",
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
			"取消待删除 来源 1 A.strm（计数 2→0/阈值 3，已确认恢复条件，文件保留）",
		},
		{
			Event{SourceIndex: 1, Kind: KindDeleted, RelativePath: "A.strm", PriorCount: 3, Reason: ReasonExcludedExtension},
			"实际删除 来源 1 A.strm（删除前计数 3/阈值 3，已实际删除，原因 扩展名排除）",
		},
		{
			Event{SourceIndex: 1, Kind: KindReclaimed, RelativePath: "A.strm"},
			"归属回收 来源 1 A.strm（本地目标此前已不存在，归属已移除）",
		},
	}
	for _, tc := range cases {
		if got := previewLine(tc.event, 3); !strings.Contains(got, tc.want) {
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
	if !strings.Contains(text, "来源总数：5（完成/未完成计数不可用）") {
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
