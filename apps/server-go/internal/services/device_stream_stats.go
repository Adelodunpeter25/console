package services

import (
	"encoding/json"
	"log/slog"
	"time"
)

// deviceStatsEvery is how often a viewer's latency window is logged.
const deviceStatsEvery = 10 * time.Second

// streamStats accumulates one viewer's server-side numbers between logs.
// Only PumpStream's writer goroutine touches it.
type streamStats struct {
	device string
	start  time.Time
	window time.Time

	frames, keys int
	bytes        int64
	queueMax     int

	// age is capture -> start of the socket write: time a picture spent in
	// sim-go's session and the viewer queue. write is the socket write.
	ageN               int
	ageSum, ageMax     time.Duration
	writeSum, writeMax time.Duration
}

func newStreamStats(device string) *streamStats {
	now := time.Now()
	return &streamStats{device: device, start: now, window: now}
}

func (s *streamStats) frame(key bool, size int, age, write time.Duration, queued int) {
	s.frames++
	if key {
		s.keys++
	}
	s.bytes += int64(size)
	if queued > s.queueMax {
		s.queueMax = queued
	}
	s.writeSum += write
	if write > s.writeMax {
		s.writeMax = write
	}
	if age > 0 { // zero for replayed pictures
		s.ageN++
		s.ageSum += age
		if age > s.ageMax {
			s.ageMax = age
		}
	}
}

// flush logs the window if it has any frames and starts a new one.
func (s *streamStats) flush(final bool) {
	secs := time.Since(s.window).Seconds()
	if s.frames > 0 && secs > 0 {
		attrs := []any{
			"device", s.device,
			"fps", float64(int(float64(s.frames)/secs*10)) / 10,
			"kbps", int(float64(s.bytes) * 8 / secs / 1000),
			"keyframes", s.keys,
			"queue_max", s.queueMax,
			"write_avg_ms", ms(s.writeSum / time.Duration(s.frames)),
			"write_max_ms", ms(s.writeMax),
		}
		if s.ageN > 0 {
			attrs = append(attrs, "age_avg_ms", ms(s.ageSum/time.Duration(s.ageN)), "age_max_ms", ms(s.ageMax))
		}
		slog.Info("device stream stats", attrs...)
	}
	if final {
		slog.Info("device stream ended", "device", s.device, "seconds", int(time.Since(s.start).Seconds()))
	}
	*s = streamStats{device: s.device, start: s.start, window: time.Now()}
}

func ms(d time.Duration) float64 { return float64(d.Microseconds()/100) / 10 }

// clientStats is what a player reports about its own decode/paint path. The
// server cannot see it, and it is where most latency hides.
type clientStats struct {
	Type         string  `json:"type"`
	Frames       int     `json:"frames"`
	DecodeAvgMs  float64 `json:"decodeAvgMs"`
	DecodeMaxMs  float64 `json:"decodeMaxMs"`
	PaintAvgMs   float64 `json:"paintAvgMs"`
	PaintMaxMs   float64 `json:"paintMaxMs"`
	Replaced     int     `json:"replaced"`
	BacklogSkips int     `json:"backlogSkips"`
	DecodeQueue  int     `json:"decodeQueueMax"`
}

// logClientStats records a player's report. It returns false when data is
// not a stats message so the caller treats it as input instead.
func logClientStats(device string, data []byte) bool {
	var c clientStats
	if json.Unmarshal(data, &c) != nil || c.Type != "stats" {
		return false
	}
	if c.Frames > 0 {
		slog.Info("device player stats",
			"device", device,
			"frames", c.Frames,
			"decode_avg_ms", c.DecodeAvgMs, "decode_max_ms", c.DecodeMaxMs,
			"paint_avg_ms", c.PaintAvgMs, "paint_max_ms", c.PaintMaxMs,
			"replaced", c.Replaced, "backlog_skips", c.BacklogSkips,
			"decode_queue_max", c.DecodeQueue,
		)
	}
	return true
}
