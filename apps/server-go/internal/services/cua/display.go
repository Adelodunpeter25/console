// X display resolution for Linux. The driver needs a reachable X server, and a
// daemon started from a service manager or SSH usually has no DISPLAY. The
// result is applied to this process only; nothing is written to ~/.console/env.
package cua

import (
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"
)

const x11SocketDir = "/tmp/.X11-unix"

// ErrNoDisplay means no X server is reachable and none could be found.
var ErrNoDisplay = errors.New("no X display is reachable for computer use: set DISPLAY to a running X server, or start a desktop session")

// EnsureDisplay makes DISPLAY point at a reachable X server on Linux, leaving
// every other platform untouched. An already-working DISPLAY always wins.
func EnsureDisplay() error {
	if runtime.GOOS != "linux" {
		return nil
	}
	display, err := resolveDisplay(os.Getenv("DISPLAY"), x11SocketDir, os.Getuid())
	if err != nil {
		return err
	}
	return os.Setenv("DISPLAY", display)
}

// DisplayPlan says which display computer use would run on, without starting
// anything. Source is "existing" (DISPLAY already works), "attached" (a running
// session was found) or "xvfb" (a private one would be started on demand).
type DisplayPlan struct {
	Display string
	Source  string
}

// PlanDisplay reports the display plan on Linux; other platforms have no
// display to plan and get a zero value.
func PlanDisplay() (DisplayPlan, error) {
	if runtime.GOOS != "linux" {
		return DisplayPlan{}, nil
	}
	return planDisplay(os.Getenv("DISPLAY"), x11SocketDir, os.Getuid(), exec.LookPath)
}

// PlanDisplayForTest exposes planning against a temporary socket directory and
// a stubbed Xvfb lookup.
func PlanDisplayForTest(current, socketDir string, uid int, lookPath func(string) (string, error)) (DisplayPlan, error) {
	return planDisplay(current, socketDir, uid, lookPath)
}

func planDisplay(current, socketDir string, uid int, lookPath func(string) (string, error)) (DisplayPlan, error) {
	display, err := resolveDisplay(current, socketDir, uid)
	if err == nil {
		source := "attached"
		if display == current {
			source = "existing"
		}
		return DisplayPlan{Display: display, Source: source}, nil
	}
	if !errors.Is(err, ErrNoDisplay) {
		return DisplayPlan{}, err
	}
	if _, lookErr := lookPath("Xvfb"); lookErr != nil {
		return DisplayPlan{}, ErrXvfbMissing
	}
	return DisplayPlan{Source: "xvfb"}, nil
}

// ResolveDisplayForTest exposes resolution so the order is pinned against a
// temporary socket directory.
func ResolveDisplayForTest(current, socketDir string, uid int) (string, error) {
	return resolveDisplay(current, socketDir, uid)
}

// resolveDisplay returns the display to use: current if it answers, otherwise
// the lowest-numbered live X socket owned by uid.
func resolveDisplay(current, socketDir string, uid int) (string, error) {
	if current != "" && displayReachable(current, socketDir) {
		return current, nil
	}
	entries, err := filepath.Glob(filepath.Join(socketDir, "X*"))
	if err != nil {
		return "", err
	}
	var nums []int
	for _, path := range entries {
		n, convErr := strconv.Atoi(strings.TrimPrefix(filepath.Base(path), "X"))
		if convErr != nil || !ownedBy(path, uid) || !dialable(path) {
			continue
		}
		nums = append(nums, n)
	}
	if len(nums) == 0 {
		if current != "" {
			return "", fmt.Errorf("DISPLAY=%s is not reachable and no other X server was found: %w", current, ErrNoDisplay)
		}
		return "", ErrNoDisplay
	}
	sort.Ints(nums)
	return ":" + strconv.Itoa(nums[0]), nil
}

// displayReachable only vouches for local displays (":N" or ":N.S"); a remote
// "host:N" cannot be probed through the socket directory and is trusted as set.
func displayReachable(display, socketDir string) bool {
	i := strings.LastIndex(display, ":")
	if i < 0 {
		return false
	}
	if i > 0 {
		return true
	}
	num := display[1:]
	if dot := strings.Index(num, "."); dot >= 0 {
		num = num[:dot]
	}
	if _, err := strconv.Atoi(num); err != nil {
		return false
	}
	return dialable(filepath.Join(socketDir, "X"+num))
}

func dialable(path string) bool {
	conn, err := net.DialTimeout("unix", path, 500*time.Millisecond)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}
