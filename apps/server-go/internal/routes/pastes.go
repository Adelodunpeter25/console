// Composer large-paste uploads: the client collapses a pasted block into a
// file-mention chip and uploads the text here; the stored file is the
// attachment the agent later reads with read_file/grep.
//
// The file must live on the server, not the client machine: the desktop can
// point at a remote daemon, and only a server-side path is visible to the
// model's tools. docs/plan/composer-large-paste.md §2.
package routes

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/run"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/utils"
)

// pasteMaxBytes bounds one upload. A 5,000-line log is tens of KB; anything
// near 2 MB defeats the point of collapsing and is rejected loudly rather
// than silently dropping lines.
const pasteMaxBytes = 2 << 20 // 2 MiB

// pasteFileStaleAfter is how long a paste file survives without the sweeper
// deciding by session liveness. Session deletion removes the directory at
// once; this only bounds files left behind a live session.
const pasteFileStaleAfter = 7 * 24 * time.Hour

// pasteSweepInterval is how often unused paste files and dead-session dirs
// are swept.
const pasteSweepInterval = time.Hour

// pasteTimestampName is the filename body layout: date + time + 4 random hex
// characters. Timestamps keep names unique across repeated pastes in one
// session without a counter file.
const pasteTimestampName = "20060102-150405"

// pasteLogLinePattern recognizes a log-ish line (timestamps, severity
// prefixes) so `.log` is an extension, not a guess taken on every upload.
var pasteLogLinePattern = regexp.MustCompile(
	`^\[?\d{4}[-/]\d{2}[-/]\d{2}[T ]\d{2}:\d{2}` +
		`|^\d{2}:\d{2}:\d{2}` +
		`|(?i)^\s*(DEBUG|INFO|WARN|ERROR|FATAL|TRACE)\b[:\s]`,
)

// RegisterPasteRoutes writes pasted text to server-side files and registers
// the hourly cleanup sweeper.
func RegisterPasteRoutes(app *fiber.App, runs *run.Service) {
	app.Post("/api/sessions/:id/pastes", func(c *fiber.Ctx) error {
		sessionID := c.Params("id")
		if !runs.SessionExists(sessionID) {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"success": false, "error": "No session '" + sessionID + "'."})
		}
		var body struct {
			Text string `json:"text"`
		}
		if err := c.BodyParser(&body); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"success": false, "error": "Invalid request body."})
		}
		if strings.TrimSpace(body.Text) == "" {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"success": false, "error": "Field 'text' is required."})
		}
		if len(body.Text) > pasteMaxBytes {
			return c.Status(fiber.StatusRequestEntityTooLarge).JSON(fiber.Map{"success": false, "error": "Pasted text is too large (2 MB limit)."})
		}

		result, err := writePaste(sessionID, body.Text)
		if err != nil {
			slog.Warn("paste upload failed", "session", sessionID, "error", err)
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"success": false, "error": "Failed to store pasted text."})
		}
		return c.JSON(fiber.Map{"success": true, "data": result})
	})

	go sweepPastes(utils.PastesDir(), runs.SessionExists)
}

// pasteResult is the response payload: name for the chip's tooltip, path for
// the mention the client inserts, counts for the label.
type pasteResult struct {
	Name  string `json:"name"`
	Path  string `json:"path"`
	Lines int    `json:"lines"`
	Bytes int    `json:"bytes"`
}

// writePaste stores text under the session's paste directory (dir 0700, file
// 0600) and returns its metadata. The path is absolute so the agent resolves
// it from any cwd.
//
// The random suffix covers the collision window. A duplicate timestamp alone
// (same second, same session) is astronomically unlikely and better retried
// than extended with counters.
func writePaste(sessionID, text string) (pasteResult, error) {
	sessionDir := utils.PastesSessionDir(sessionID)
	if err := os.MkdirAll(sessionDir, 0o700); err != nil {
		return pasteResult{}, fmt.Errorf("create paste dir: %w", err)
	}

	body := pasteNameBase()
	ext := ".txt"
	if logLines := countLogLines(text); logLines >= 2 {
		// At least two timestamp/severity lines: logs, not prose or JSON.
		ext = ".log"
	}
	lines := countLines(text)

	suffix := make([]byte, 2)
	var path string
	for attempt := 0; attempt < 3; attempt++ {
		if _, err := rand.Read(suffix); err != nil {
			return pasteResult{}, err
		}
		name := fmt.Sprintf("%s-%s%s", body, hex.EncodeToString(suffix), ext)
		candidate := filepath.Join(sessionDir, name)
		if _, statErr := os.Stat(candidate); statErr == nil {
			continue
		}
		path = candidate
		break
	}
	if path == "" {
		return pasteResult{}, fmt.Errorf("could not pick a unique paste name")
	}

	if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
		return pasteResult{}, err
	}
	name := filepath.Base(path)
	return pasteResult{Name: name, Path: path, Lines: lines, Bytes: len(text)}, nil
}

// pasteNameBase is the shared filename prefix, without extension: the
// timestamp plus a random suffix slot.
func pasteNameBase() string {
	return "pasted-" + time.Now().Format(pasteTimestampName)
}

// countLines counts '\n'-separated lines the way a text editor does: a
// trailing newline does not start a new empty line.
func countLines(text string) int {
	lines := strings.Count(text, "\n") + 1
	if strings.HasSuffix(text, "\n") {
		lines--
	}
	return lines
}

// countLogLines counts lines matching the log-ish pattern; used for the
// extension heuristic.
func countLogLines(text string) int {
	count := 0
	for i := 0; i < len(text); i++ {
		lineEnd := i
		for lineEnd < len(text) && text[lineEnd] != '\n' {
			lineEnd++
		}
		if pasteLogLinePattern.MatchString(text[i:lineEnd]) {
			count++
		}
		i = lineEnd
	}
	return count
}

// sweepPastes removes stale paste files and the paste directories of
// sessions that no longer exist. Started once with the daemon; runs forever.
// The directory is captured at registration so a later CONSOLE_PASTES_DIR
// change (tests, dev switches) can never redirect the sweeper at a foreign
// storage tree.
func sweepPastes(dir string, sessionExists func(string) bool) {
	ticker := time.NewTicker(pasteSweepInterval)
	defer ticker.Stop()
	for {
		<-ticker.C
		sweepOnce(dir, sessionExists)
	}
}

// SweepPastesOnce exposes one sweeper pass for tests.
func SweepPastesOnce(dir string, sessionExists func(string) bool) {
	sweepOnce(dir, sessionExists)
}

func sweepOnce(dir string, sessionExists func(string) bool) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	now := time.Now()
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		sessionID := entry.Name()
		path := filepath.Join(dir, sessionID)
		// A session that no longer exists (deleted, or the daemon was reset
		// with a fresh DB) has nothing left that can reference its paste
		// file — drop the whole directory regardless of age.
		if sessionExists != nil && !sessionExists(sessionID) {
			_ = os.RemoveAll(path)
			continue
		}
		files, err := os.ReadDir(path)
		if err != nil {
			continue
		}
		staleOnly := len(files) > 0
		for _, file := range files {
			if file.IsDir() {
				staleOnly = false
				continue
			}
			full := filepath.Join(path, file.Name())
			info, statErr := file.Info()
			if statErr != nil {
				staleOnly = false
				continue
			}
			if now.Sub(info.ModTime()) > pasteFileStaleAfter {
				if unlinkErr := os.Remove(full); unlinkErr != nil {
					staleOnly = false
				}
				continue
			}
			staleOnly = false
		}
		if staleOnly || len(files) == 0 {
			_ = os.Remove(path)
		}
	}
}
