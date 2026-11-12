package notify

import (
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
		"1 来源 1 实际删除 M/A.strm 删除前计数 3/阈值 3 原因 源目标未读到",
		"2 来源 1 归属回收 M/B.strm 原因 扩展名排除",
		"3 来源 2 取消待删除 N/C.strm 计数 2→0/阈值 3",
		"4 来源 2 缺失计数 N/D.strm 计数 1→2/阈值 3 原因 扩展名排除",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("listing missing %q:\n%s", want, content)
		}
	}
}
