package tests

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	terminalsvc "github.com/Adelodunpeter25/console/apps/server-go/internal/services/terminal"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/types"
)

var oscTitleRe = regexp.MustCompile("\x1b\\]0;([^\x07]*)\x07")

func (o *outputSink) titles() []string {
	o.mu.Lock()
	defer o.mu.Unlock()
	var out []string
	for _, m := range oscTitleRe.FindAllStringSubmatch(o.buf.String(), -1) {
		out = append(out, m[1])
	}
	return out
}

func (o *outputSink) hasTitle(pred func(string) bool) bool {
	for _, title := range o.titles() {
		if pred(title) {
			return true
		}
	}
	return false
}

func checkShellTitles(t *testing.T, shell, marker string) {
	if _, err := os.Stat(shell); err != nil {
		t.Skipf("%s not present", shell)
	}
	m := terminalsvc.NewPtyManager(nil, nil)
	defer m.KillAll()

	dir := t.TempDir()
	session, err := m.Spawn(types.TerminalSpawnParams{Cwd: dir, Shell: shell, Cols: 80, Rows: 24})
	if err != nil {
		t.Fatal(err)
	}
	sink := &outputSink{}
	session.Attach(sink.write, func(int) {})

	base := filepath.Base(dir)
	waitFor(t, func() bool {
		return sink.hasTitle(func(s string) bool { return strings.Contains(s, base) })
	})

	_ = session.Write([]byte("echo " + marker + "\n"))
	want := "echo " + marker
	waitFor(t, func() bool {
		return sink.hasTitle(func(s string) bool { return s == want })
	})

	for _, title := range sink.titles() {
		if strings.Contains(title, "PROMPT_COMMAND") || strings.Contains(title, "__console") {
			t.Fatalf("setup command leaked into title: %q", title)
		}
	}
}

func TestShellIntegrationTitlesZsh(t *testing.T)  { checkShellTitles(t, "/bin/zsh", "zsh_ok") }
func TestShellIntegrationTitlesBash(t *testing.T) { checkShellTitles(t, "/bin/bash", "bash_ok") }

func TestShellIntegrationPlainShStillWorks(t *testing.T) {
	m := terminalsvc.NewPtyManager(nil, nil)
	defer m.KillAll()
	session, err := m.Spawn(types.TerminalSpawnParams{Cwd: t.TempDir(), Shell: "/bin/sh", Cols: 80, Rows: 24})
	if err != nil {
		t.Fatal(err)
	}
	sink := &outputSink{}
	session.Attach(sink.write, func(int) {})
	_ = session.Write([]byte("echo plain_$((1+1))\n"))
	waitFor(t, func() bool { return sink.contains("plain_2") })
}
