package tests

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The module is github.com/Adelodunpeter25/console. A missing "25" in an import
// path compiles nowhere and, because the error names a package rather than the
// typo, is easy to misread. This fails with the offending file instead.
// misspelled is assembled at run time so this file does not contain it.
var misspelled = "github.com/Adelodunpeter" + "/console"

func TestNoImportUsesAMisspelledModulePath(t *testing.T) {
	root := filepath.Join("..", "..")
	var bad []string
	_ = filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(p, ".go") || strings.Contains(p, "third_party") {
			return nil
		}
		b, rerr := os.ReadFile(p)
		if rerr == nil && strings.Contains(string(b), misspelled) {
			bad = append(bad, p)
		}
		return nil
	})
	if len(bad) > 0 {
		t.Fatalf("these files import %s (missing the 25 in the module name): %v", misspelled, bad)
	}
}
