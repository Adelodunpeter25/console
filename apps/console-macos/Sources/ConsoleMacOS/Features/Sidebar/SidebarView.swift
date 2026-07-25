import SwiftUI
import ConsoleCore

struct SidebarView: View {
    @ObservedObject var app: AppViewModel
    @Binding var selectedSessionId: String?
    @Environment(\.theme) private var theme

    @State private var showingNewSession = false
    @State private var newSessionCwd = ""
    @State private var newSessionTitle = ""
    @AppStorage("sidebar.order") private var orderRaw = "updated"

    private var order: SidebarOrder { SidebarOrder(rawValue: orderRaw) ?? .updated }

    private var sortedSessions: [SessionHeader] {
        app.sessions.sorted { left, right in
            switch order {
            case .updated:
                return left.updatedAt > right.updatedAt
            case .created:
                return left.createdAt > right.createdAt
            }
        }
    }

    var body: some View {
        VStack(spacing: 0) {
            // MARK: - Pinned header

            VStack(alignment: .leading, spacing: 1) {
                actionRow("New Session", systemImage: "plus.square.on.square") {
                    showingNewSession = true
                }
                sessionsHeader
            }
            .padding(.horizontal, 8)
            .padding(.top, 8)

            // MARK: - Scrollable session list

            ScrollView {
                VStack(alignment: .leading, spacing: 1) {
                    if sortedSessions.isEmpty {
                        Text("No sessions yet")
                            .font(.caption)
                            .foregroundStyle(.tertiary)
                            .padding(.horizontal, 10)
                            .padding(.vertical, 4)
                    } else {
                        ForEach(sortedSessions) { session in
                            sessionRow(session)
                        }
                    }
                }
                .padding(.horizontal, 8)
                .padding(.bottom, 8)
            }
            .scrollContentBackground(.hidden)
        }
        .background(.regularMaterial)
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

    // MARK: - Header rows

    private func actionRow(_ title: String, systemImage: String, action: @escaping () -> Void) -> some View {
        HStack(spacing: 8) {
            Image(systemName: systemImage)
                .frame(width: 18)
                .foregroundStyle(.secondary)
            Text(title)
                .foregroundStyle(Color.primary)
                .lineLimit(1)
            Spacer(minLength: 0)
        }
        .padding(.horizontal, 8)
        .padding(.vertical, 6)
        .frame(maxWidth: .infinity, alignment: .leading)
        .contentShape(Rectangle())
        .onTapGesture(perform: action)
    }

    private var sessionsHeader: some View {
        HStack {
            Text("Sessions")
                .font(.subheadline.weight(.semibold))
                .foregroundStyle(.secondary)
            Spacer()
            Menu {
                Picker("Order by", selection: Binding(
                    get: { order },
                    set: { orderRaw = $0.rawValue }
                )) {
                    ForEach(SidebarOrder.allCases, id: \.self) { option in
                        Text(option.title).tag(option)
                    }
                }
            } label: {
                Image(systemName: "line.3.horizontal.decrease")
                    .font(.callout.weight(.semibold))
                    .foregroundStyle(.secondary)
            }
            .menuStyle(.button)
            .buttonStyle(.plain)
            .help("Sort sessions")
            .accessibilityLabel("Sort sessions")
        }
        .padding(.horizontal, 10)
        .padding(.top, 12)
        .padding(.bottom, 4)
    }

    // MARK: - Session row

    private func sessionRow(_ session: SessionHeader) -> some View {
        let isSelected = selectedSessionId == session.id
        return HoverableRow(isSelected: isSelected) { isHovered in
            HStack(spacing: 7) {
                Image(systemName: "bubble.left.and.bubble.right")
                    .font(.caption)
                    .frame(width: 18)
                    .foregroundStyle(.secondary)

                VStack(alignment: .leading, spacing: 1) {
                    Text(session.title)
                        .font(.body)
                        .lineLimit(1)
                    Text("\(session.modelId) · \(session.provider)")
                        .font(.caption2)
                        .foregroundStyle(.tertiary)
                        .lineLimit(1)
                }
                .frame(maxWidth: .infinity, alignment: .leading)

                Text(RelativeTime.short(from: session.updatedAt))
                    .font(.caption2)
                    .foregroundStyle(.tertiary)
            }
            .padding(.horizontal, 8)
            .padding(.vertical, 5)
            .frame(maxWidth: .infinity, alignment: .leading)
            .contentShape(Rectangle())
            .foregroundStyle(isSelected ? Color.primary : Color.secondary)
            .onTapGesture {
                selectedSessionId = session.id
            }
        }
        .contextMenu {
            Button {
                Task { await app.deleteSession(id: session.id) }
            } label: {
                Label("Delete", systemImage: "trash")
                    .labelStyle(.titleAndIcon)
            }
        }
    }
}

// MARK: - HoverableRow

private struct HoverableRow<Content: View>: View {
    var isSelected = false
    @ViewBuilder var content: (_ isHovered: Bool) -> Content
    @Environment(\.theme) private var theme
    @State private var isHovered = false

    var body: some View {
        let revealsHoverContent = isHovered
        content(revealsHoverContent)
            .background(
                RoundedRectangle(cornerRadius: 6)
                    .fill(isSelected ? theme.rowSelectedBackground
                        : (isHovered ? theme.rowHoverBackground : Color.clear))
            )
            .onHover { isHovered = $0 }
    }
}

// MARK: - Sort order

private enum SidebarOrder: String, CaseIterable {
    case updated
    case created

    var title: String {
        switch self {
        case .updated: return "Last updated"
        case .created: return "Created"
        }
    }
}

// MARK: - Relative time

enum RelativeTime {
    static func short(from timestamp: UInt64) -> String {
        let date = Date(timeIntervalSince1970: TimeInterval(timestamp))
        return short(from: date)
    }

    static func short(from date: Date, now: Date = Date()) -> String {
        let seconds = max(0, now.timeIntervalSince(date))
        switch seconds {
        case ..<60: return "now"
        case ..<3600: return "\(Int(seconds / 60))m"
        case ..<86_400: return "\(Int(seconds / 3600))h"
        default: return "\(Int(seconds / 86_400))d"
        }
    }
}

// MARK: - New Session Sheet

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
