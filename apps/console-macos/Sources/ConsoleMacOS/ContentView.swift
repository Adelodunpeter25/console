import SwiftUI
import ConsoleCore

struct ContentView: View {
    @StateObject private var app = AppViewModel()
    @State private var selectedSessionId: String?
    @State private var showingSettings = false

    var body: some View {
        NavigationSplitView {
            SidebarView(
                app: app,
                selectedSessionId: $selectedSessionId
            )
            .navigationSplitViewColumnWidth(min: 220, ideal: 260, max: 360)
        } detail: {
            if let sessionId = selectedSessionId {
                ChatView(
                    app: app,
                    sessionId: sessionId,
                    onSwitchProject: { project in
                        Task {
                            if let session = await app.createSession(
                                cwd: project.path,
                                projectId: project.id,
                                title: project.name
                            ) {
                                selectedSessionId = session.id
                            }
                        }
                    }
                )
            } else {
                ContentUnavailableViewCompat("No Session Selected") {
                    Image(systemName: "bubble.left.and.bubble.right")
                } description: {
                    Text("Select a session from the sidebar or create a new one.")
                }
            }
        }
        .frame(minWidth: 800, minHeight: 500)
        .themedRoot()
        .environmentObject(app)
        .task {
            await app.initialLoad()
        }
        .sheet(isPresented: $showingSettings) {
            SettingsView(app: app)
        }
        .toolbar {
            ToolbarItem(placement: .primaryAction) {
                Button {
                    showingSettings = true
                } label: {
                    Label("Settings", systemImage: "gearshape")
                }
            }
        }
    }
}
