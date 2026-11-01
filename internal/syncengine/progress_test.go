package syncengine

import (
	"fmt"
	"testing"
	"time"

	"ssh_gun/internal/config"
)

func TestProgressLog_EmitsEveryBatch(t *testing.T) {
	clock := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	logs := captureProgress()
	p := newTestProgressLog("docs", &clock, logs)

	for i := 0; i < progressBatchSize-1; i++ {
		p.Record(Upload, fmt.Sprintf("f%d.txt", i), 10)
	}
	if logs.count() != 0 {
		t.Fatalf("logged before batch: %#v", logs.msgs)
	}

	p.Record(Upload, "f49.txt", 10)
	if logs.count() != 1 {
		t.Fatalf("got %d logs after first batch, want 1", logs.count())
	}
	got := logs.last()
	if got.files != progressBatchSize || got.latest != "f49.txt" || got.bytes != int64(progressBatchSize*10) {
		t.Fatalf("first batch = %+v", got)
	}

	for i := 0; i < progressBatchSize-1; i++ {
		p.Record(Upload, fmt.Sprintf("g%d.txt", i), 1)
	}
	if logs.count() != 1 {
		t.Fatalf("logged before second batch: %d", logs.count())
	}
	p.Record(Upload, "g49.txt", 1)
	if logs.count() != 2 {
		t.Fatalf("got %d logs after second batch, want 2", logs.count())
	}
}

func TestProgressLog_EmitsAfterIntervalWithNewWork(t *testing.T) {
	clock := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	logs := captureProgress()
	p := newTestProgressLog("docs", &clock, logs)

	p.Record(Upload, "a.txt", 4)
	p.Record(Mkdir, "sub", 0)
	p.Record(Delete, "old.txt", 0)
	if logs.count() != 0 {
		t.Fatalf("logged before interval: %#v", logs.msgs)
	}

	clock = clock.Add(progressInterval)
	p.Record(Upload, "b.txt", 6)
	if logs.count() != 1 {
		t.Fatalf("got %d logs after interval, want 1", logs.count())
	}
	got := logs.last()
	if got.files != 2 || got.dirs != 1 || got.deleted != 1 || got.bytes != 10 || got.latest != "b.txt" {
		t.Fatalf("interval log = %+v", got)
	}

	p.Record(Upload, "c.txt", 1)
	if logs.count() != 1 {
		t.Fatalf("logged again without waiting: %d", logs.count())
	}
}

func TestProgressLog_DoesNotLogWhenNeitherConditionMet(t *testing.T) {
	clock := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	logs := captureProgress()
	p := newTestProgressLog("docs", &clock, logs)

	for i := 0; i < 10; i++ {
		p.Record(Upload, fmt.Sprintf("n%d.txt", i), 1)
		clock = clock.Add(100 * time.Millisecond)
	}
	if logs.count() != 0 {
		t.Fatalf("logged without batch or 2s idle-with-work window: %#v", logs.msgs)
	}
}

func TestProgressLog_NoteBytesThrottlesLargeFile(t *testing.T) {
	clock := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	logs := captureProgress()
	p := newTestProgressLog("docs", &clock, logs)

	p.NoteBytes("big.bin", 100)
	if logs.count() != 0 {
		t.Fatalf("logged on first byte tick: %#v", logs.msgs)
	}

	clock = clock.Add(progressInterval)
	p.NoteBytes("big.bin", 200)
	if logs.count() != 1 {
		t.Fatalf("got %d logs after interval, want 1", logs.count())
	}
	got := logs.last()
	if got.files != 0 || got.bytes != 200 || got.latest != "big.bin" {
		t.Fatalf("in-flight log = %+v", got)
	}

	p.NoteBytes("big.bin", 300)
	if logs.count() != 1 {
		t.Fatalf("logged again within interval: %d", logs.count())
	}

	p.Record(Upload, "big.bin", 1000)
	if logs.count() != 1 {
		t.Fatalf("completed file logged immediately: %d", logs.count())
	}
	clock = clock.Add(progressInterval)
	p.Record(Upload, "next.txt", 5)
	if logs.count() != 2 {
		t.Fatalf("got %d logs after completed interval, want 2", logs.count())
	}
	got = logs.last()
	if got.files != 2 || got.bytes != 1005 || got.latest != "next.txt" {
		t.Fatalf("completed interval log = %+v", got)
	}
}

func TestProgressLog_BeginLogsImmediately(t *testing.T) {
	clock := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	logs := captureProgress()
	p := newTestProgressLog("docs", &clock, logs)

	p.Begin("scan-local")
	if logs.count() != 1 {
		t.Fatalf("got %d logs after Begin, want 1", logs.count())
	}
	got := logs.last()
	if got.phase != "scan-local" || got.files != 0 || got.latest != "" {
		t.Fatalf("begin log = %+v", got)
	}

	for i := 0; i < progressBatchSize-1; i++ {
		p.Tick(fmt.Sprintf("d%d", i), true)
	}
	if logs.count() != 1 {
		t.Fatalf("logged before scan batch: %d", logs.count())
	}
	p.Tick("keep/file.txt", false)
	if logs.count() != 2 {
		t.Fatalf("got %d logs after scan batch, want 2", logs.count())
	}
	got = logs.last()
	if got.phase != "scan-local" || got.dirs != progressBatchSize-1 || got.files != 1 || got.latest != "keep/file.txt" {
		t.Fatalf("scan batch = %+v", got)
	}
}

func TestProgressLog_HeartbeatAndSetScan(t *testing.T) {
	clock := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	logs := captureProgress()
	p := newTestProgressLog("docs", &clock, logs)

	p.Begin("scan-remote")
	p.Heartbeat()
	if logs.count() != 1 {
		t.Fatalf("heartbeat before interval = %d", logs.count())
	}

	clock = clock.Add(progressInterval)
	p.Heartbeat()
	if logs.count() != 2 {
		t.Fatalf("got %d logs after heartbeat interval, want 2", logs.count())
	}
	if logs.last().phase != "scan-remote" {
		t.Fatalf("heartbeat phase = %+v", logs.last())
	}

	p.SetScan(12, 4, "src/main.go")
	if logs.count() != 3 {
		t.Fatalf("got %d logs after SetScan, want 3", logs.count())
	}
	got := logs.last()
	if got.files != 12 || got.dirs != 4 || got.latest != "src/main.go" || got.phase != "scan-remote" {
		t.Fatalf("set scan = %+v", got)
	}
}

func TestMappingLogName(t *testing.T) {
	if got := MappingLogName(nil); got != "" {
		t.Fatalf("nil mapping = %q", got)
	}
	named := config.SyncMapping{Name: "docs", ID: "id-1"}
	if got := MappingLogName(&named); got != "docs" {
		t.Fatalf("name = %q", got)
	}
	unnamed := config.SyncMapping{ID: "id-1"}
	if got := MappingLogName(&unnamed); got != "id-1" {
		t.Fatalf("id fallback = %q", got)
	}
}

type progressLogCapture struct {
	msgs []progressLogMsg
}

type progressLogMsg struct {
	mapping string
	phase   string
	files   int
	dirs    int
	deleted int
	bytes   int64
	latest  string
}

func captureProgress() *progressLogCapture {
	return &progressLogCapture{}
}

func (c *progressLogCapture) count() int { return len(c.msgs) }

func (c *progressLogCapture) last() progressLogMsg {
	return c.msgs[len(c.msgs)-1]
}

func newTestProgressLog(mapping string, clock *time.Time, logs *progressLogCapture) *ProgressLog {
	p := NewProgressLog(mapping)
	p.now = func() time.Time { return *clock }
	p.lastLog = *clock
	p.logf = func(_ string, args ...any) {
		logs.msgs = append(logs.msgs, progressLogMsg{
			mapping: attrString(args, "mapping"),
			phase:   attrString(args, "phase"),
			files:   attrInt(args, "files"),
			dirs:    attrInt(args, "dirs"),
			deleted: attrInt(args, "deleted"),
			bytes:   attrInt64(args, "bytes"),
			latest:  attrString(args, "latest"),
		})
	}
	return p
}

func attrString(args []any, key string) string {
	v, _ := attrValue(args, key).(string)
	return v
}

func attrInt(args []any, key string) int {
	switch v := attrValue(args, key).(type) {
	case int:
		return v
	case int64:
		return int(v)
	default:
		return 0
	}
}

func attrInt64(args []any, key string) int64 {
	switch v := attrValue(args, key).(type) {
	case int64:
		return v
	case int:
		return int64(v)
	default:
		return 0
	}
}

func attrValue(args []any, key string) any {
	for i := 0; i+1 < len(args); i += 2 {
		if args[i] == key {
			return args[i+1]
		}
	}
	return nil
}
