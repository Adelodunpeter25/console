import SwiftUI
import AppKit

@main
struct ConsoleMacOSApp: App {
    init() {
        // SPM executables don't have a bundle icon — set one from SF Symbols.
        let config = NSImage.SymbolConfiguration(pointSize: 128, weight: .regular)
        NSApp.applicationIconImage = NSImage(
            systemSymbolName: "terminal.fill",
            accessibilityDescription: "Console"
        )?.withSymbolConfiguration(config)
    }

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
