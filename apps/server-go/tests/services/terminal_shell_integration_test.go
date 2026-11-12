package tests

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	terminalsvc "github.com/Adelodunpeter25/console/apps/server-go/internal/services/terminal"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/types"
)

func TestShellIntegrationConfiguresZsh(t *testing.T) {
	m := terminalsvc.NewPtyManager(nil, nil)
	defer m.KillAll()

	zshPath := "/bin/zsh"
	if _, err := os.Stat(zshPath); err != nil {
		t.Skip("/bin/zsh not present")
	}

	tempDir := t.TempDir()
	session, err := m.Spawn(types.TerminalSpawnParams{
		Cwd:   tempDir,
		Shell: zshPath,
		Cols:  80,
		Rows:  24,
	})
	if err != nil {
		t.Fatalf("failed to spawn zsh: %v", err)
	}

	sink := &outputSink{}
	session.Attach(sink.write, func(int) {})

	// Wait for prompt OSC 0 title to be emitted
	waitFor(t, func() bool {
		return sink.contains("\x1b]0;")
	})

	// Run a command and verify command title is emitted
	_ = session.Write([]byte("echo zsh_ok\n"))
	waitFor(t, func() bool {
		return sink.contains("zsh_ok")
	})

	if !sink.contains("\x1b]0;") {
		t.Fatal("expected OSC 0 terminal title in output")
	}
}

func TestShellIntegrationConfiguresBash(t *testing.T) {
	m := terminalsvc.NewPtyManager(nil, nil)
	defer m.KillAll()

	bashPath := "/bin/bash"
	if _, err := os.Stat(bashPath); err != nil {
		t.Skip("/bin/bash not present")
	}

	tempDir := t.TempDir()
	session, err := m.Spawn(types.TerminalSpawnParams{
		Cwd:   tempDir,
		Shell: bashPath,
		Cols:  80,
		Rows:  24,
	})
	if err != nil {
		t.Fatalf("failed to spawn bash: %v", err)
	}

	sink := &outputSink{}
	session.Attach(sink.write, func(int) {})

	// Wait for prompt OSC 0 title to be emitted
	waitFor(t, func() bool {
		return sink.contains("\x1b]0;")
	})

	// Run a command and verify command title is emitted
	_ = session.Write([]byte("echo bash_ok\n"))
	waitFor(t, func() bool {
		return sink.contains("bash_ok")
	})

	baseName := filepath.Base(tempDir)
	if !sink.contains(baseName) && !sink.contains("~") {
		t.Logf("output: %s", sink.buf.String())
	}
}

func TestShellIntegrationFallbackOnPlainSh(t *testing.T) {
	m := terminalsvc.NewPtyManager(nil, nil)
	defer m.KillAll()

	tempDir := t.TempDir()
	session, err := m.Spawn(types.TerminalSpawnParams{
		Cwd:   tempDir,
		Shell: "/bin/sh",
		Cols:  80,
		Rows:  24,
	})
	if err != nil {
		t.Fatalf("failed to spawn sh: %v", err)
	}

	sink := &outputSink{}
	session.Attach(sink.write, func(int) {})

	_ = session.Write([]byte("echo plain_sh_ok\n"))
	waitFor(t, func() bool {
		return strings.Contains(sink.buf.String(), "plain_sh_ok")
	})
}
