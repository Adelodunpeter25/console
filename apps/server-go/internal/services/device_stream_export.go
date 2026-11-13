package services

import "time"

// Test hooks: the tests live in tests/, outside this package.

type StreamStatsForTest struct{ s *streamStats }

func NewStreamStatsForTest(device string) *StreamStatsForTest {
	return &StreamStatsForTest{s: newStreamStats(device)}
}

func (t *StreamStatsForTest) Frame(key bool, size int, age, write time.Duration, queued int) {
	t.s.frame(key, size, age, write, queued)
}

func (t *StreamStatsForTest) Flush(final bool) { t.s.flush(final) }

func LogClientStatsForTest(device string, data []byte) bool { return logClientStats(device, data) }
