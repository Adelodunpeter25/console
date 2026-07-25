import SwiftUI
import GhosttyTerminal

/// A Ghostty terminal pane that can be embedded in the session view.
///
/// Uses `libghostty-spm`'s `TerminalViewState` + `TerminalSurfaceView` for
/// a Metal-backed terminal with full IME, clipboard, scrollback, and
/// shell-integration support.
///
/// Two modes:
///   - `.local` — Ghostty launches the user's shell (`.exec` backend)
///   - `.hosted` — the app owns the PTY byte stream (`.inMemory` backend,
///      for server-managed terminals)
struct TerminalPaneView: View {
    let mode: TerminalMode
    var workingDirectory: String? = nil
    var fontSize: Float = 13
    var fontFamily: String = "SF Mono"

    @StateObject private var terminal: TerminalViewState
    @FocusState private var terminalFocused: Bool
    @State private var title: String = "Terminal"

    enum TerminalMode {
        case local
        case hosted
    }

    init(
        mode: TerminalMode = .local,
        workingDirectory: String? = nil,
        fontSize: Float = 13,
        fontFamily: String = "SF Mono"
    ) {
        self.mode = mode
        self.workingDirectory = workingDirectory
        self.fontSize = fontSize
        self.fontFamily = fontFamily

        let config = TerminalConfiguration.default
            .fontFamily(fontFamily)
            .fontSize(fontSize)
            .windowPaddingX(8)
            .windowPaddingY(6)
            .custom("shell-integration", "detect")

        _terminal = StateObject(
            wrappedValue: TerminalViewState(
                terminalConfiguration: config
            )
        )
    }

    var body: some View {
        VStack(spacing: 0) {
            TerminalSurfaceView(context: terminal)
                .terminalFocusOnAppear($terminalFocused)
                .onAppear {
                    configureTerminal()
                }
        }
        .navigationTitle(title)
    }

    private func configureTerminal() {
        let options = TerminalSurfaceOptions(
            backend: .exec,
            workingDirectory: workingDirectory ?? FileManager.default.homeDirectoryForCurrentUser.path,
            envVars: ["TERM_EMBEDDED_BY": "Console"]
        )
        terminal.configuration = options
    }
}

/// A compact terminal pane with a header bar showing the terminal title
/// and a close button. Designed for embedding in a split/tab layout.
struct TerminalPaneCard: View {
    let mode: TerminalPaneView.TerminalMode
    var workingDirectory: String? = nil
    var onClose: (() -> Void)? = nil

    var body: some View {
        VStack(spacing: 0) {
            // Header bar
            HStack(spacing: 8) {
                Image(systemName: "terminal")
                    .font(.caption)
                    .foregroundStyle(.secondary)

                Text(terminalTitle)
                    .font(.caption.weight(.medium))
                    .foregroundStyle(.secondary)
                    .lineLimit(1)

                Spacer()

                if let onClose {
                    Button(action: onClose) {
                        Image(systemName: "xmark")
                            .font(.caption2.weight(.semibold))
                            .foregroundStyle(.tertiary)
                    }
                    .buttonStyle(.plain)
                    .help("Close terminal")
                }
            }
            .padding(.horizontal, 10)
            .padding(.vertical, 6)
            .background(Color(nsColor: .controlBackgroundColor))

            Divider()

            // Terminal surface
            TerminalPaneView(
                mode: mode,
                workingDirectory: workingDirectory
            )
        }
        .background(Color.black.opacity(0.9))
        .clipShape(RoundedRectangle(cornerRadius: 8, style: .continuous))
    }

    private var terminalTitle: String {
        if let workingDirectory {
            return (workingDirectory as NSString).lastPathComponent
        }
        return "Terminal"
    }
}
