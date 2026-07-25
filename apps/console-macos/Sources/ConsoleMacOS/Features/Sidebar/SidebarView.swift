import SwiftUI
import ConsoleCore

struct SidebarView: View {
    @ObservedObject var app: AppViewModel
    @Binding var selectedSessionId: String?
    @State private var showingNewSession = false
    @State private var newSessionCwd = ""
    @State private var newSessionTitle = ""

    var body: some View {
        List(selection: $selectedSessionId) {
            if app.sessions.isEmpty {
                ContentUnavailableViewCompat("No Sessions") {
                    Image(systemName: "tray")
                } description: {
                    Text("Create a session to get started.")
                }
                .frame(maxWidth: .infinity, maxHeight: .infinity)
            } else {
                Section("Sessions") {
                    ForEach(app.sessions) { session in
                        SessionRow(session: session)
                            .tag(session.id)
                    }
                }
            }
        }
        .listStyle(.sidebar)
        .navigationTitle("Console")
        .toolbar {
            ToolbarItem(placement: .primaryAction) {
                Button {
                    showingNewSession = true
                } label: {
                    Label("New Session", systemImage: "plus")
                }
            }
        }
        .sheet(isPresented: $showingNewSession) {
            NewSessionSheet(
                cwd: $newSessionCwd,
                title: $newSessionTitle,
                projects: app.projects,
                onCreate: {
                    Task {
                        if let session = await app.createSession(
                            cwd: newSessionCwd.isEmpty ? FileManager.default.currentDirectoryPath : newSessionCwd,
                            title: newSessionTitle.isEmpty ? nil : newSessionTitle
                        ) {
                            selectedSessionId = session.id
                        }
                        showingNewSession = false
                        newSessionCwd = ""
                        newSessionTitle = ""
                    }
                },
                onCancel: {
                    showingNewSession = false
                    newSessionCwd = ""
                    newSessionTitle = ""
                }
            )
        }
        .refreshable {
            await app.refreshSessions()
        }
    }
}

private struct SessionRow: View {
    let session: SessionHeader

    var body: some View {
        VStack(alignment: .leading, spacing: 2) {
            Text(session.title)
                .font(.body)
                .lineLimit(1)
            HStack(spacing: 6) {
                Text(session.modelId)
                    .font(.caption)
                    .foregroundStyle(.secondary)
                Text("·")
                    .font(.caption)
                    .foregroundStyle(.tertiary)
                Text(session.provider)
                    .font(.caption)
                    .foregroundStyle(.secondary)
            }
        }
        .padding(.vertical, 2)
    }
}

private struct NewSessionSheet: View {
    @Binding var cwd: String
    @Binding var title: String
    let projects: [ProjectInfo]
    let onCreate: () -> Void
    let onCancel: () -> Void

    var body: some View {
        VStack(alignment: .leading, spacing: 16) {
            Text("New Session")
                .font(.headline)

            if !projects.isEmpty {
                Picker("Project", selection: $cwd) {
                    Text("Custom path").tag("")
                    ForEach(projects) { project in
                        Text(project.name).tag(project.path)
                    }
                }
                .pickerStyle(.menu)
            }

            TextField("Working directory", text: $cwd, prompt: Text("/path/to/project"))
                .textFieldStyle(.roundedBorder)

            TextField("Title (optional)", text: $title)
                .textFieldStyle(.roundedBorder)

            HStack {
                Button("Cancel", action: onCancel)
                    .keyboardShortcut(.cancelAction)
                Spacer()
                Button("Create", action: onCreate)
                    .keyboardShortcut(.defaultAction)
                    .disabled(cwd.isEmpty && projects.isEmpty)
            }
        }
        .padding(20)
        .frame(width: 420)
    }
}
