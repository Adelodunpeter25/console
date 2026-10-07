// The `console computer-use` command family: status, a manual probe, and the
// macOS bundle builder that gives the runtime a stable identity to hold TCC
// grants against.
package commands

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/services/cua"
)

// ComputerUseCommand builds the `computer-use` command tree.
func ComputerUseCommand() *cobra.Command {
	root := &cobra.Command{
		Use:   "computer-use",
		Short: "Inspect and exercise the Cua Driver computer-use runtime",
	}
	root.AddCommand(computerUseStatusCmd())
	root.AddCommand(computerUsePermissionsCmd())
	root.AddCommand(computerUseProbeCmd())
	root.AddCommand(computerUseRunCmd())
	root.AddCommand(computerUseEvalCmd())
	root.AddCommand(computerUseBundleCmd())
	root.AddCommand(computerUseResetCmd())
	return root
}

// computerUsePermissionsCmd asks the driver itself, rather than inferring from
// whether a tool happened to work. The driver reports the live TCC state and,
// crucially, which process identity it was attributed to: "host" is correct,
// "caller" means the process is running outside its bundle and borrowing
// someone else's grant.
func computerUsePermissionsCmd() *cobra.Command {
	var out string
	cmd := &cobra.Command{
		Use:   "permissions",
		Short: "Report the live macOS Accessibility and Screen Recording state for this process",
		Long: `Report the live macOS Accessibility and Screen Recording state for this process.

macOS attributes a grant to the responsible process, not to a binary path. Run
from a shell, that process is your terminal, so this reports the terminal's
grants. To check the bundle's own grants, launch it through Launch Services:

  open "/Applications/Console Computer Use.app" --args computer-use permissions --out /tmp/perm.txt`,
		RunE: func(_ *cobra.Command, _ []string) error {
			if runtime.GOOS != "darwin" {
				return fmt.Errorf("only macOS has permission grants to report; this is %s", runtime.GOOS)
			}
			cua.EnsureHostEnv()
			// Launch Services gives the process no terminal, so output goes to
			// a file when one is requested.
			restore := redirectOutput(out)
			defer restore()

			driver, err := cua.Open(cua.Options{})
			if err != nil {
				return err
			}
			defer driver.Shutdown()
			res, err := driver.Call(context.Background(), "check_permissions", map[string]any{}, nil)
			if err != nil {
				return err
			}
			printResult(res)
			return explainAttribution(res)
		},
	}
	cmd.Flags().StringVar(&out, "out", "", "Write the report to this file instead of stdout")
	return cmd
}

// redirectOutput sends stdout and stderr to path when one is given, returning a
// function that restores them. An empty path is a no-op.
func redirectOutput(path string) func() {
	if path == "" {
		return func() {}
	}
	file, err := os.Create(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "could not open %s: %v\n", path, err)
		return func() {}
	}
	stdout, stderr := os.Stdout, os.Stderr
	os.Stdout, os.Stderr = file, file
	return func() {
		os.Stdout, os.Stderr = stdout, stderr
		file.Close()
	}
}

// explainAttribution turns the driver's attribution report into the one action
// that matters, because "permissions are granted but they belong to the wrong
// process" is otherwise very confusing.
func explainAttribution(res *cua.ToolResult) error {
	if len(res.StructuredContent) == 0 {
		return nil
	}
	var payload struct {
		Accessibility *bool `json:"accessibility"`
		ScreenRecord  *bool `json:"screen_recording"`
		Source        struct {
			Attribution     string `json:"attribution"`
			HostBundleID    string `json:"host_bundle_id"`
			Embedded        bool   `json:"embedded"`
			ResponsiblePPID int    `json:"responsible_ppid"`
			Executable      string `json:"executable"`
		} `json:"source"`
	}
	if json.Unmarshal(res.StructuredContent, &payload) != nil {
		return nil
	}

	fmt.Printf("\nattribution: %s\n", payload.Source.Attribution)
	if payload.Source.HostBundleID != "" {
		fmt.Printf("  host bundle: %s\n", payload.Source.HostBundleID)
	}
	if payload.Source.Executable != "" {
		fmt.Printf("  executable:  %s\n", payload.Source.Executable)
	}

	granted := payload.Accessibility != nil && *payload.Accessibility
	attribution := payload.Source.Attribution
	if granted && attribution == "host" {
		fmt.Println("\nAccessibility is granted to this bundle. Reading windows should work.")
		return nil
	}
	if granted {
		fmt.Println("\nAccessibility is granted, but NOT to this process identity.")
		fmt.Println("  Run the server from inside the bundle so it inherits the grant:")
		fmt.Println("    open \"/Applications/Console Computer Use.app\"")
		return nil
	}
	fmt.Println("\nAccessibility is not granted. Add the bundle in System Settings:")
	fmt.Println("  Privacy & Security → Accessibility → + → Console Computer Use")
	fmt.Println("  Privacy & Security → Screen Recording → + → Console Computer Use")
	return nil
}

func computerUseStatusCmd() *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Report whether computer use is available, and why not if it is not",
		RunE: func(_ *cobra.Command, _ []string) error {
			status := cua.NewManager().Status()
			if asJSON {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(status)
			}
			if status.Error != "" {
				fmt.Printf("computer use: unavailable\n  %s\n", status.Error)
				if status.LibraryPath == "" {
					fmt.Println("  set CUA_DRIVER_LIB_PATH, or place libcua_driver_sdk next to the console binary")
				} else {
					fmt.Printf("  looked for: %s\n", status.LibraryPath)
				}
				return nil
			}
			fmt.Printf("computer use: available\n")
			if status.ABIMajor > 0 {
				fmt.Printf("  ABI %d.%d.%d, permission mode %s\n",
					status.ABIMajor, status.ABIMinor, status.ABIPatch, status.PermissionMode)
			}
			if len(status.Tools) > 0 {
				fmt.Printf("  %d tools\n", len(status.Tools))
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "Emit the raw status payload")
	return cmd
}

// computerUseProbeCmd drives one real observation so the capability can be
// checked end to end without going through a chat. This is the manual
// equivalent of what /computer-use will do.
func computerUseProbeCmd() *cobra.Command {
	var app, query, out string
	cmd := &cobra.Command{
		Use:   "probe",
		Short: "Launch an app, read its window, and report what came back",
		RunE: func(_ *cobra.Command, _ []string) error {
			cua.EnsureHostEnv()
			restore := redirectOutput(out)
			defer restore()
			if app == "" {
				app = "com.apple.calculator"
			}
			driver, err := cua.Open(cua.Options{})
			if err != nil {
				return err
			}
			defer driver.Shutdown()
			ctx := context.Background()

			fmt.Printf("launching %s…\n", app)
			res, err := driver.Call(ctx, "launch_app", map[string]any{"bundle_id": app}, nil)
			if err != nil {
				return err
			}
			printResult(res)

			pid := intFromJSON(res.StructuredContent, "pid")
			if pid == 0 {
				return fmt.Errorf("launch did not return a pid")
			}

			windowID, err := waitForWindow(ctx, driver, pid)
			if err != nil {
				return err
			}
			fmt.Printf("\nwindow %d on pid %d\n\n", windowID, pid)

			args := map[string]any{"pid": pid, "window_id": windowID}
			if query != "" {
				args["query"] = query
			}
			res, err = driver.Call(ctx, "get_window_state", args, nil)
			if err != nil {
				return err
			}
			printResult(res)
			reportCapture(res)
			return nil
		},
	}
	cmd.Flags().StringVar(&app, "app", "", "Bundle identifier to launch (default com.apple.calculator)")
	cmd.Flags().StringVar(&query, "query", "", "Narrow the accessibility read to elements matching this text")
	cmd.Flags().StringVar(&out, "out", "", "Write the report to this file instead of stdout (needed when launched via open)")
	return cmd
}

// waitForWindow polls list_windows because a freshly launched app reports
// window_ready=false: the window does not exist at the instant launch returns.
func waitForWindow(ctx context.Context, driver *cua.Driver, pid int) (uint64, error) {
	for attempt := 0; attempt < 20; attempt++ {
		res, err := driver.Call(ctx, "list_windows", map[string]any{"pid": pid}, nil)
		if err != nil {
			return 0, err
		}
		var payload struct {
			Windows []struct {
				WindowID uint64 `json:"window_id"`
				Title    string `json:"title"`
			} `json:"windows"`
		}
		if len(res.StructuredContent) > 0 {
			_ = json.Unmarshal(res.StructuredContent, &payload)
		}
		if len(payload.Windows) > 0 {
			return payload.Windows[0].WindowID, nil
		}
		sleepMillis(500)
	}
	return 0, fmt.Errorf("the app never reported a window on pid %d", pid)
}

func printResult(res *cua.ToolResult) {
	if res.IsError {
		fmt.Printf("  [error] %s\n", firstText(res))
	}
	for _, part := range res.Content {
		switch part.Type {
		case "text":
			fmt.Printf("  %s\n", part.Text)
		case "image":
			fmt.Printf("  [image %s, %d bytes]\n", part.MIMEType, len(part.Data))
		}
	}
	if len(res.StructuredContent) > 0 {
		fmt.Printf("  structured: %s\n", truncate(string(res.StructuredContent), 400))
	}
}

// reportCapture explains an empty observation, because "0 elements" and "the
// grant is missing" look identical otherwise.
func reportCapture(res *cua.ToolResult) {
	var state struct {
		Elements   int   `json:"element_count"`
		FrameValid *bool `json:"screenshot_frame_valid"`
		ShotWidth  int   `json:"screenshot_width"`
	}
	if len(res.StructuredContent) > 0 {
		_ = json.Unmarshal(res.StructuredContent, &state)
	}
	if state.Elements > 0 && (state.FrameValid == nil || *state.FrameValid) {
		return
	}
	fmt.Println("\nThis observation is empty. That is what a missing macOS grant looks like:")
	fmt.Println("  build the bundle:   console computer-use bundle")
	fmt.Println("  then grant it in System Settings → Privacy & Security → Accessibility")
	fmt.Println("  and → Screen Recording, then re-run. Check with: console computer-use status")
}

func intFromJSON(raw []byte, key string) int {
	var payload map[string]any
	if json.Unmarshal(raw, &payload) != nil {
		return 0
	}
	switch v := payload[key].(type) {
	case float64:
		return int(v)
	case int:
		return v
	}
	return 0
}

func firstText(res *cua.ToolResult) string {
	for _, part := range res.Content {
		if part.Type == "text" {
			return part.Text
		}
	}
	return "(no message)"
}

func sleepMillis(n int) { time.Sleep(time.Duration(n) * time.Millisecond) }

func copyStream(dst io.Writer, src io.Reader) (int64, error) { return io.Copy(dst, src) }

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// BundleIdentity is the macOS bundle the runtime must own to hold TCC grants.
const (
	bundleName       = "Console Computer Use.app"
	bundleIdentifier = "sh.console.computeruse"
)

// computerUseBundleCmd builds the .app bundle. It is a directory with a plist
// containing the server binary: macOS attributes Accessibility and Screen
// Recording to the bundle identifier, not to an executable path, so the binary
// is copied in unchanged and no rebuild is required.
func computerUseBundleCmd() *cobra.Command {
	var dest, identity string
	cmd := &cobra.Command{
		Use:   "bundle",
		Short: "Build and sign the macOS bundle that holds the computer-use permissions",
		RunE: func(_ *cobra.Command, _ []string) error {
			if runtime.GOOS != "darwin" {
				return fmt.Errorf("the bundle is only needed on macOS; this is %s", runtime.GOOS)
			}
			if dest == "" {
				dest = defaultBundleDir()
			}
			if identity == "" {
				identity = "-"
			}
			return BuildBundle(dest, identity)
		},
	}
	cmd.Flags().StringVar(&dest, "dest", "", "Where to build the bundle (default next to the console binary)")
	cmd.Flags().StringVar(&identity, "identity", "-", "codesign identity; \"-\" is ad-hoc, which is fine locally")
	return cmd
}

// defaultBundleDir puts the bundle beside the console binary so the binary
// inside is the same one already on disk.
func defaultBundleDir() string {
	if exe, err := os.Executable(); err == nil {
		return filepath.Join(filepath.Dir(exe), bundleName)
	}
	return bundleName
}

// BuildBundle writes the bundle, copies the current executable in, and signs it.
func BuildBundle(dest, identity string) error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("could not locate the console binary: %w", err)
	}
	exe, err = filepath.EvalSymlinks(exe)
	if err != nil {
		return err
	}

	macOSDir := filepath.Join(dest, "Contents", "MacOS")
	if err := os.MkdirAll(macOSDir, 0o755); err != nil {
		return err
	}

	// The binary goes in unchanged. Its identity for TCC purposes is the
	// bundle identifier in the plist, so nothing about it needs rebuilding.
	target := filepath.Join(macOSDir, filepath.Base(exe))
	if err := copyFile(exe, target); err != nil {
		return err
	}

	// The driver library travels beside the binary so it is found without an
	// environment override. It has to exist before signing: codesign refuses a
	// bundle whose nested code is unsigned.
	if source, err := cua.LibraryPathOverride(); err == nil && source != "" {
		if _, statErr := os.Stat(source); statErr == nil {
			library := filepath.Join(macOSDir, filepath.Base(source))
			if filepath.Clean(library) != filepath.Clean(source) {
				if err := copyFile(source, library); err != nil {
					return err
				}
				signed, err := signPath(library, identity)
				if err != nil {
					return err
				}
				fmt.Printf("  library: %s\n", signed)
			}
		}
	}

	plistPath := filepath.Join(dest, "Contents", "Info.plist")
	if err := os.WriteFile(plistPath, []byte(bundlePlist(filepath.Base(exe))), 0o644); err != nil {
		return err
	}

	// Signing is what makes the bundle addressable for TCC. A new signature
	// orphans an existing grant, so the identity has to be stable once real
	// permissions are granted.
	if _, err := signPath(target, identity); err != nil {
		return err
	}
	if _, err := signPath(dest, identity); err != nil {
		return err
	}

	fmt.Printf("built %s\n", dest)
	fmt.Printf("  binary:  %s\n", target)
	fmt.Printf("  bundle:  %s\n", bundleIdentifier)
	fmt.Printf("  signed:  %s\n", strings.TrimSpace(identity))
	fmt.Println("\nNext:")
	fmt.Println("  1. Grant it in System Settings → Privacy & Security → Accessibility")
	fmt.Println("  2. Grant it in System Settings → Privacy & Security → Screen Recording")
	fmt.Println("  3. Start the daemon through the bundle so it inherits the identity:")
	fmt.Printf("       open %q\n", dest)
	fmt.Println("  4. Check with: console computer-use status")
	return nil
}

// signPath ad-hoc or Developer-ID signs one path. Nested code must be signed
// before its container, or codesign rejects the bundle outright.
func signPath(path, identity string) (string, error) {
	out, err := exec.Command("codesign", "--force", "--sign", identity, path).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("codesign %s failed: %v\n%s", path, err, out)
	}
	return path, nil
}

func bundlePlist(executable string) string {
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>CFBundleDevelopmentRegion</key>
	<string>en</string>
	<key>CFBundleExecutable</key>
	<string>%s</string>
	<key>CFBundleIdentifier</key>
	<string>%s</string>
	<key>CFBundleName</key>
	<string>%s</string>
	<key>CFBundlePackageType</key>
	<string>APPL</string>
	<key>CFBundleShortVersionString</key>
	<string>1.0</string>
	<key>CFBundleVersion</key>
	<string>1</string>
	<key>LSMinimumSystemVersion</key>
	<string>13.0</string>
	<key>NSAccessibilityUsageDescription</key>
	<string>Console uses Accessibility access to operate apps on your behalf when you ask it to.</string>
	<key>NSScreenCaptureUsageDescription</key>
	<string>Console uses Screen Recording access to read the screen for the app you asked it to work with.</string>
</dict>
</plist>
`, executable, bundleIdentifier, "Console Computer Use")
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = copyStream(out, in)
	return err
}

// computerUseResetCmd clears the bundle's TCC rows. macOS caches grant answers
// per process, so a development loop needs this to re-prompt deliberately.
func computerUseResetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "reset-permissions",
		Short: "Clear this bundle's Accessibility and Screen Recording grants",
		RunE: func(_ *cobra.Command, _ []string) error {
			if runtime.GOOS != "darwin" {
				return fmt.Errorf("TCC grants only exist on macOS; this is %s", runtime.GOOS)
			}
			for _, service := range []string{"Accessibility", "ScreenCapture"} {
				// tccutil reports failure when no row exists, which is not an
				// error here: the goal is that the row is absent afterwards.
				out, _ := exec.Command("tccutil", "reset", service, bundleIdentifier).CombinedOutput()
				fmt.Printf("%s: %s\n", service, strings.TrimSpace(string(out)))
			}
			fmt.Printf("\nRestart the daemon after granting again; macOS caches answers per process.\n")
			return nil
		},
	}
}

// BundlePlistForTest exposes the generated plist so its load-bearing fields can
// be asserted without writing to /Applications.
func BundlePlistForTest(executable string) string { return bundlePlist(executable) }
