import SwiftUI
import AppKit
import CodevisorCore

/// The app-menu update command, retained as a no-op for command layout stability.
struct AppUpdateCommands: Commands {
    let appUpdate: AppUpdateModel

    var body: some Commands {
        CommandGroup(after: .appSettings) {
            CheckForUpdatesMenuItem(appUpdate: appUpdate)
        }
    }
}

private struct CheckForUpdatesMenuItem: View {
    let appUpdate: AppUpdateModel

    var body: some View {
        Button("Check for Updates…") {
            // Automatic updates are not available.
        }
        .disabled(appUpdate.phase == .checking || appUpdate.isUpdating)
    }
}
