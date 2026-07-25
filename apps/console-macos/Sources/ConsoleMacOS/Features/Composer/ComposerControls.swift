import SwiftUI
import ConsoleCore

/// The combined model dropdown: one chip that opens a menu of models grouped
/// by provider. A native checkmark marks the currently selected model;
/// "Default" clears the selection so the server picks. The chip reads the
/// selected model id (or "Default").
///
/// Replaces Console's old separate model + provider pickers: selecting a
/// model implicitly determines its provider, matching Codevisor's single
/// ModelConfigMenu chip.
struct ModelConfigMenu: View {
    let providers: [ProviderCatalogEntry]
    @Binding var selectedModelId: String?

    var body: some View {
        if providers.isEmpty {
            // Nothing to show yet — the catalog hasn't loaded.
            EmptyView()
        } else {
            Menu {
                Toggle(isOn: Binding(
                    get: { selectedModelId == nil },
                    set: { isOn in if isOn { selectedModelId = nil } }
                )) {
                    Text("Default")
                }
                Divider()
                ForEach(providers) { provider in
                    Section(provider.displayName) {
                        ForEach(provider.models) { model in
                            Toggle(isOn: Binding(
                                get: { model.id == selectedModelId },
                                set: { isOn in if isOn { selectedModelId = model.id } }
                            )) {
                                Text(model.id)
                            }
                        }
                    }
                }
            } label: {
                chipLabel
            }
            .menuStyle(.button)
            .buttonStyle(HoverIconButtonStyle(shape: .chip))
            .menuIndicator(.hidden)
            .fixedSize()
            .help("Model")
            .accessibilityLabel("Model")
            .accessibilityValue(selectedModelId ?? "Default")
        }
    }

    private var chipLabel: some View {
        HStack(spacing: 5) {
            Image(systemName: "cpu")
                .font(.caption)
                .foregroundStyle(.secondary)
            Text(selectedModelId ?? "Default")
                .foregroundStyle(.primary)
        }
        .contentShape(Rectangle())
    }
}

/// The project picker chip. Choosing a project creates a new session under
/// that project (sessions are project-scoped on the Console backend), so the
/// picker drives `onSelect` rather than mutating the current session.
struct ProjectPickerMenu: View {
    let projects: [ProjectInfo]
    let selectedProjectId: String?
    let onSelect: (ProjectInfo) -> Void

    var body: some View {
        if projects.isEmpty {
            EmptyView()
        } else {
            Menu {
                ForEach(projects) { project in
                    Toggle(isOn: Binding(
                        get: { project.id == selectedProjectId },
                        set: { isOn in if isOn { onSelect(project) } }
                    )) {
                        Text(project.name)
                    }
                }
            } label: {
                HStack(spacing: 5) {
                    Image(systemName: "folder")
                        .font(.caption)
                        .foregroundStyle(.secondary)
                    Text(selectedName)
                        .foregroundStyle(.primary)
                }
                .contentShape(Rectangle())
            }
            .menuStyle(.button)
            .buttonStyle(HoverIconButtonStyle(shape: .chip))
            .menuIndicator(.hidden)
            .fixedSize()
            .help("Project")
            .accessibilityLabel("Project")
            .accessibilityValue(selectedName)
        }
    }

    private var selectedName: String {
        projects.first { $0.id == selectedProjectId }?.name ?? "Choose project"
    }
}

/// The primary send action: a circular filled button with a brighten-on-hover
/// treatment. Adapted from Codevisor's ComposerSubmitButton (no Liquid Glass).
struct ComposerSubmitButton: View {
    @Environment(\.theme) private var theme
    let isEnabled: Bool
    let help: String
    let accessibilityLabel: String
    let action: () -> Void

    @State private var isHovered = false

    var body: some View {
        Button(action: action) {
            Image(systemName: "arrow.up")
                .font(.system(size: 12, weight: .bold))
                .frame(width: 26, height: 26)
                .foregroundStyle(isEnabled ? theme.windowBackground : Color.secondary.opacity(0.75))
                .background(
                    Circle().fill(
                        isEnabled
                            ? Color.primary.opacity(isHovered ? 0.92 : 0.82)
                            : Color.secondary.opacity(0.16)
                    )
                )
                .contentShape(Circle())
        }
        .buttonStyle(.plain)
        .disabled(!isEnabled)
        .onHover { isHovered = $0 }
        .help(help)
        .accessibilityLabel(accessibilityLabel)
    }
}

/// The stop button shown while a run is streaming. Quiet circular chrome with
/// a hover wash, matching the send button's geometry.
struct ComposerStopButton: View {
    let action: () -> Void

    @State private var isHovered = false

    var body: some View {
        Button(action: action) {
            Image(systemName: "stop.fill")
                .font(.system(size: 10, weight: .bold))
                .frame(width: 26, height: 26)
                .background(
                    Circle().fill(isHovered ? Color.primary.opacity(0.06) : .clear)
                )
                .overlay(
                    Circle()
                        .strokeBorder(Color.secondary.opacity(isHovered ? 0.55 : 0.35), lineWidth: 1)
                )
                .contentShape(Circle())
        }
        .buttonStyle(.plain)
        .foregroundStyle(isHovered ? .primary : .secondary)
        .onHover { isHovered = $0 }
        .help("Stop")
        .accessibilityLabel("Stop")
    }
}
