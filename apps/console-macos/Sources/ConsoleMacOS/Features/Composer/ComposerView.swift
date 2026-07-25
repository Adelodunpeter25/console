import SwiftUI
import ConsoleCore

/// The chat composer card: an auto-growing plain-text input (Return sends,
/// Shift+Return adds a newline) with an inline toolbar holding the project
/// picker, the combined model picker, and a send/stop button.
///
/// Adapted from Codevisor's ComposerCard for macOS 13 / Swift 5.9 and the
/// HTTP ConsoleCore architecture — no Liquid Glass, harness, slash-command
/// palette, or attachment strip. The project picker creates a new session
/// under the chosen project via `onSwitchProject`; the model picker selects
/// the model (and its implicit provider) for the next send.
struct ComposerView: View {
    let isStreaming: Bool
    let onSend: (String, String?, String?) -> Void
    let onAbort: () -> Void
    let providers: [ProviderCatalogEntry]
    let projects: [ProjectInfo]
    let selectedProjectId: String?
    let onSwitchProject: (ProjectInfo) -> Void

    @Environment(\.theme) private var theme
    @Environment(\.accessibilityReduceMotion) private var reduceMotion

    @State private var prompt = ""
    @State private var selectedModelId: String?
    @State private var editorHeight: CGFloat = ChatInputEditor.singleLineHeight

    var body: some View {
        VStack(alignment: .leading, spacing: 10) {
            inputArea

            HStack(spacing: 10) {
                ProjectPickerMenu(
                    projects: projects,
                    selectedProjectId: selectedProjectId,
                    onSelect: onSwitchProject
                )
                ModelConfigMenu(
                    providers: providers,
                    selectedModelId: $selectedModelId
                )
                Spacer(minLength: 0)
                actionButtons
            }
            .font(.callout)
        }
        .padding(12)
        .background(
            RoundedRectangle(cornerRadius: 16, style: .continuous)
                .fill(theme.composerBackground)
        )
        .overlay(
            RoundedRectangle(cornerRadius: 16, style: .continuous)
                .strokeBorder(theme.border, lineWidth: 1)
        )
        .padding(.horizontal, 12)
        .padding(.bottom, 12)
        .animation(reduceMotion ? nil : .easeInOut(duration: 0.15), value: isStreaming)
    }

    // MARK: - Input

    private var inputArea: some View {
        ZStack(alignment: .topLeading) {
            ChatInputEditor(
                text: $prompt,
                calculatedHeight: $editorHeight,
                onSubmit: send,
                onKeyCommand: nil
            )
            .frame(height: editorHeight)
            .disabled(isStreaming)

            if prompt.isEmpty {
                Text("Do anything")
                    .foregroundStyle(.tertiary)
                    .padding(.top, 6)
                    .allowsHitTesting(false)
            }
        }
    }

    // MARK: - Action buttons

    @ViewBuilder
    private var actionButtons: some View {
        if isStreaming {
            ComposerStopButton(action: onAbort)
        } else {
            ComposerSubmitButton(
                isEnabled: hasDraft,
                help: hasDraft ? "Send (↩)" : "Type a message to send",
                accessibilityLabel: "Send",
                action: send
            )
        }
    }

    private var hasDraft: Bool {
        !prompt.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty
    }

    // MARK: - Send

    private func send() {
        let trimmed = prompt.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !trimmed.isEmpty else { return }
        let provider = providerForModel(selectedModelId)
        onSend(trimmed, selectedModelId, provider)
        prompt = ""
    }

    /// Resolve the provider name for a selected model by looking it up in the
    /// catalog. The combined model menu selects a model id; its provider is
    /// implicit, so we send both to satisfy `RunPromptDto`.
    private func providerForModel(_ modelId: String?) -> String? {
        guard let modelId else { return nil }
        for provider in providers {
            if provider.models.contains(where: { $0.id == modelId }) {
                return provider.name
            }
        }
        return nil
    }
}
