// Live output for a background bash job: replaces client polling with a push
// of each new chunk as the job writes it.
package terminal

import (
	"time"
)

const (
	// bashStreamChunk is the most stdout one frame carries; a bigger backlog
	// is sent as consecutive frames without waiting.
	bashStreamChunk = 50_000
	// bashStreamBatch coalesces a burst of writes into one frame.
	bashStreamBatch = 50 * time.Millisecond
	// bashStreamTailGrace lets the last bytes of a killed or timed-out job
	// land before the closing frame is read.
	bashStreamTailGrace = 150 * time.Millisecond
	// BashStreamPingInterval spaces heartbeats across silent stretches.
	BashStreamPingInterval = 15 * time.Second
)

// BashJobChunk is one pushed update: new stdout since the previous cursor, the
// stderr tail when it changed, and the job's current status.
type BashJobChunk struct {
	Snapshot   BashJobSnapshot
	Stdout     string
	Stderr     *string // nil when unchanged since the last chunk
	NextCursor int
	Truncated  bool
}

// StreamOutput pushes a job's output to emit until the job settles, starting
// at cursor. The first chunk always goes out (it carries the current status),
// then one per batch of new output or status change, and a closing chunk once
// the job is no longer running. ping is called every BashStreamPingInterval
// while nothing happens. A non-nil error from emit or ping (client gone) ends
// the stream.
func (m *BashJobManager) StreamOutput(jobID, ownerSessionID string, cursor int, emit func(BashJobChunk) error, ping func() error) error {
	rec, err := m.getOwned(jobID, ownerSessionID)
	if err != nil {
		return err
	}
	ticker := time.NewTicker(BashStreamPingInterval)
	defer ticker.Stop()

	var (
		lastStderr string
		sentFirst  bool
		lastStatus BashJobStatus
	)
	for {
		// Take the wake channel before reading so a write that lands between
		// the read and the wait is never missed.
		wake := rec.changedCh()
		out, err := m.Output(jobID, ownerSessionID, cursor, bashStreamChunk)
		if err != nil {
			return err
		}
		terminal := out.Snapshot.Status != BashJobRunning
		if terminal && (out.Snapshot.Aborted || out.Snapshot.TimedOut) {
			time.Sleep(bashStreamTailGrace)
			if again, err := m.Output(jobID, ownerSessionID, cursor, bashStreamChunk); err == nil {
				out = again
			}
		}

		var stderr *string
		if out.Stderr != lastStderr {
			s := out.Stderr
			stderr = &s
			lastStderr = out.Stderr
		}
		// Output clamps the cursor down to what exists so far when a resumed
		// client is ahead of the buffer. A stream cursor only moves forward,
		// or the next read would resend everything already delivered.
		if out.NextCursor < cursor {
			out.NextCursor = cursor
		}
		if !sentFirst || out.Stdout != "" || stderr != nil || out.Snapshot.Status != lastStatus {
			if err := emit(BashJobChunk{
				Snapshot: out.Snapshot, Stdout: out.Stdout, Stderr: stderr,
				NextCursor: out.NextCursor, Truncated: out.Truncated,
			}); err != nil {
				return err
			}
			sentFirst = true
			lastStatus = out.Snapshot.Status
		}
		cursor = out.NextCursor
		if terminal && len(out.Stdout) < bashStreamChunk {
			return nil
		}
		// A full chunk means more is already buffered: keep reading.
		if len(out.Stdout) >= bashStreamChunk {
			continue
		}

		for waiting := true; waiting; {
			select {
			case <-wake:
				time.Sleep(bashStreamBatch)
				waiting = false
			case <-ticker.C:
				if err := ping(); err != nil {
					return err
				}
			}
		}
	}
}
