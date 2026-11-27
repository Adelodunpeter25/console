package tests

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/services"
)

func TestStreamStatsLogWindowAndClientReport(t *testing.T) {
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, nil)))
	defer slog.SetDefault(prev)

	s := services.NewStreamStatsForTest("iPhone 17")
	s.Frame(true, 5000, 12*time.Millisecond, time.Millisecond, 3)
	s.Frame(false, 1000, 8*time.Millisecond, 2*time.Millisecond, 1)
	s.Frame(false, 1000, 0, time.Millisecond, 0) // replayed: no age
	time.Sleep(20 * time.Millisecond)
	s.Flush(false)

	out := buf.String()
	for _, want := range []string{`"device stream stats"`, `"iPhone 17"`, `"keyframes":1`, `"queue_max":3`, `"age_max_ms":12`} {
		if !strings.Contains(out, want) {
			t.Errorf("log missing %s:\n%s", want, out)
		}
	}

	buf.Reset()
	if !services.LogClientStatsForTest("iPhone 17", []byte(`{"type":"stats","frames":30,"decodeAvgMs":4.5,"paintAvgMs":1.2,"backlogSkips":2}`)) {
		t.Fatal("stats message must be consumed")
	}
	if !strings.Contains(buf.String(), `"device player stats"`) || !strings.Contains(buf.String(), `"backlog_skips":2`) {
		t.Fatalf("client log: %s", buf.String())
	}
	if services.LogClientStatsForTest("x", []byte(`{"type":"touch","action":"down"}`)) {
		t.Fatal("input must not be treated as stats")
	}
}
