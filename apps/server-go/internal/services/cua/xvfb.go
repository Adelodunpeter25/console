// A private headless X server for machines with no display (servers, CI, SSH
// sessions). It exists only for the life of the Manager that started it.
package cua

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ErrXvfbMissing means there is no display and no Xvfb to make one.
var ErrXvfbMissing = fmt.Errorf("%w: install Xvfb (apt install xvfb, dnf install xorg-x11-server-Xvfb) to run computer use headless", ErrNoDisplay)

const (
	xvfbScreen       = "1280x800x24"
	xvfbStartTimeout = 10 * time.Second
)

type xvfb struct {
	cmd     *exec.Cmd
	display string
	once    sync.Once
}

// startXvfb launches bin and waits for it to report the display number it
// picked. -displayfd lets the server choose a free number itself, so there is
// no hard-coded display and no race with another server.
func startXvfb(bin string, timeout time.Duration) (*xvfb, error) {
	r, w, err := os.Pipe()
	if err != nil {
		return nil, err
	}
	defer r.Close()
	cmd := exec.Command(bin, "-displayfd", "3", "-screen", "0", xvfbScreen, "-nolisten", "tcp")
	cmd.ExtraFiles = []*os.File{w}
	err = cmd.Start()
	w.Close()
	if err != nil {
		return nil, fmt.Errorf("start Xvfb: %w", err)
	}
	x := &xvfb{cmd: cmd}

	type result struct {
		line string
		err  error
	}
	got := make(chan result, 1)
	go func() {
		line, err := bufio.NewReader(r).ReadString('\n')
		got <- result{line, err}
	}()
	select {
	case res := <-got:
		num := strings.TrimSpace(res.line)
		if _, convErr := strconv.Atoi(num); res.err != nil || convErr != nil {
			x.Stop()
			return nil, errors.New("Xvfb exited before reporting a display")
		}
		x.display = ":" + num
		return x, nil
	case <-time.After(timeout):
		x.Stop()
		return nil, errors.New("Xvfb did not report a display in time")
	}
}

// Display is the X display name, e.g. ":101".
func (x *xvfb) Display() string { return x.display }

// Stop ends the server and reaps it. Safe to call more than once.
func (x *xvfb) Stop() {
	x.once.Do(func() {
		if x.cmd.Process != nil {
			_ = x.cmd.Process.Kill()
		}
		_ = x.cmd.Wait()
	})
}

// StartXvfbForTest exposes startup against a stand-in binary.
func StartXvfbForTest(bin string, timeout time.Duration) (display string, stop func(), err error) {
	x, err := startXvfb(bin, timeout)
	if err != nil {
		return "", nil, err
	}
	return x.Display(), x.Stop, nil
}
