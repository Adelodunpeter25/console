import SwiftUI
import AppKit

@main
struct ConsoleMacOSApp: App {
    var body: some Scene {
        WindowGroup {
            ContentView()
                .frame(minWidth: 900, minHeight: 600)
                .onAppear {
                    // SPM executables don't have a bundle icon — set one from
                    // SF Symbols. Must run after NSApp is initialized.
                    if NSApp.applicationIconImage == nil,
                       let base = NSImage(
                        systemSymbolName: "terminal.fill",
                        accessibilityDescription: "Console"
                       ) {
                        let config = NSImage.SymbolConfiguration(
                            pointSize: 128, weight: .regular
                        )
                        NSApp.applicationIconImage =
                            base.withSymbolConfiguration(config) ?? base
                    }
                }
        }
        .windowStyle(.titleBar)
        .defaultSize(width: 1280, height: 820)
        .windowResizability(.contentMinSize)
        .commands {
            CommandGroup(replacing: .newItem) {}
        }
    }
}
