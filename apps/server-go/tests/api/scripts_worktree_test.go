// Scripts follow the checkout they are asked about: a worktree of the project
// reads its own console.toml and runs in its own folder, while anything that
// is not the project or one of its worktrees falls back to the project folder.
package tests

import (
	"path/filepath"
	"testing"
	"time"
)

func TestScriptsReadAndRunInWorktree(t *testing.T) {
	svc, projectID, root := newScriptService(t)
	gitIn(t, root, "init", "-b", "main")
	writeConsoleToml(t, root, "[scripts.where]\nlabel = \"Main\"\ncommand = \"pwd\"\n")
	gitIn(t, root, "add", "console.toml")
	gitIn(t, root, "-c", "user.name=t", "-c", "user.email=t@t", "commit", "-m", "init")
	worktree := filepath.Join(t.TempDir(), "wt")
	gitIn(t, root, "worktree", "add", "-b", "feature", worktree)
	// The worktree's own console.toml differs from main's.
	writeConsoleToml(t, worktree, "[scripts.where]\nlabel = \"Worktree\"\ncommand = \"pwd\"\n")

	main, err := svc.ListIn(projectID, "")
	if err != nil || len(main.Scripts) != 1 || main.Scripts[0].Label != "Main" {
		t.Fatalf("main list: %+v %v", main, err)
	}
	wt, err := svc.ListIn(projectID, worktree)
	if err != nil || len(wt.Scripts) != 1 || wt.Scripts[0].Label != "Worktree" {
		t.Fatalf("worktree list: %+v %v", wt, err)
	}

	run, err := svc.RunIn(projectID, "where", worktree)
	if err != nil {
		t.Fatalf("run in worktree: %v", err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for svc.IsRunning(projectID, run.RunID) && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	final := svc.GetRun(projectID, run.RunID)
	if final == nil || !scriptOutputContains(final.Stdout, filepath.Base(worktree)) {
		t.Fatalf("script should have run in the worktree, got %+v", final)
	}
}

func TestScriptsIgnoreFoldersOutsideTheProject(t *testing.T) {
	svc, projectID, root := newScriptService(t)
	gitIn(t, root, "init", "-b", "main")
	writeConsoleToml(t, root, "[scripts.where]\nlabel = \"Main\"\ncommand = \"pwd\"\n")

	// An unrelated folder with its own console.toml must never be used.
	other := t.TempDir()
	writeConsoleToml(t, other, "[scripts.evil]\nlabel = \"Evil\"\ncommand = \"pwd\"\n")
	for _, cwd := range []string{other, filepath.Join(root, "sub"), "/", "relative/path"} {
		res, err := svc.ListIn(projectID, cwd)
		if err != nil || len(res.Scripts) != 1 || res.Scripts[0].ID != "where" {
			t.Fatalf("cwd %q should fall back to the project folder: %+v %v", cwd, res, err)
		}
	}
}
