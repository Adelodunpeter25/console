import SwiftUI
import AppKit

// MARK: - AppKit-based application delegate

/// We use an `NSApplicationDelegateAdaptor` instead of a plain SwiftUI `App`
/// so we can set the activation policy to `.regular` (dock icon + menu bar)
/// and set the application icon before any SwiftUI content mounts.
/// SPM executables don't have an Info.plist or bundle icon, so without this
/// the app shows no dock icon.
final class AppDelegate: NSObject, NSApplicationDelegate {
    func applicationDidFinishLaunching(_ notification: Notification) {
        // .regular = dock icon + standard app menu bar
        NSApp.setActivationPolicy(.regular)

        // Set dock icon from SF Symbols (SPM executables have no bundle icon)
        if let base = NSImage(
            systemSymbolName: "terminal.fill",
            accessibilityDescription: "Console"
        ) {
            let config = NSImage.SymbolConfiguration(
                pointSize: 128, weight: .regular
            )
            NSApp.applicationIconImage =
                base.withSymbolConfiguration(config) ?? base
        }

        // Bring the window to front
        NSApp.activate(ignoringOtherApps: true)
    }
}

@main
struct ConsoleMacOSApp: App {
    @NSApplicationDelegateAdaptor(AppDelegate.self) var appDelegate

    var body: some Scene {
        WindowGroup {
            ContentView()
                .frame(minWidth: 900, minHeight: 600)
        }
        .windowStyle(.titleBar)
        .defaultSize(width: 1280, height: 820)
        .windowResizability(.contentMinSize)
        .commands {
            CommandGroup(replacing: .newItem) {}
        }
    }
}
