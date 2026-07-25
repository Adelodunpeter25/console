import SwiftUI
import ConsoleCore

struct ChatView: View {
    @ObservedObject var app: AppViewModel
    let sessionId: String
    @StateObject private var sessionVM: SessionViewModel

    init(app: AppViewModel, sessionId: String) {
        self.app = app
        self.sessionId = sessionId
        _sessionVM = StateObject(wrappedValue: SessionViewModel(client: app.client, sessionId: sessionId))
    }

    var body: some View {
        VStack(spacing: 0) {
            messageList
            Divider()
            if let perm = sessionVM.pendingPermission {
                PermissionBanner(perm: perm, onApprove: { Task { await sessionVM.approveTool(allow: true) } }, onDeny: { Task { await sessionVM.approveTool(allow: false) } })
            }
            if let question = sessionVM.pendingQuestion {
                QuestionBanner(question: question, onAnswer: { answer in Task { await sessionVM.answerQuestion(answer) } })
            }
            ComposerView(
                isStreaming: sessionVM.isStreaming,
                onSend: { prompt, modelId, provider in
                    Task { await sessionVM.sendPrompt(prompt, modelId: modelId, provider: provider) }
                },
                onAbort: { Task { await sessionVM.abort() } },
                providers: app.providers
            )
        }
        .task {
            await sessionVM.loadDetails()
        }
        .id(sessionId)
    }

    private var messageList: some View {
        ScrollViewReader { proxy in
            ScrollView {
                LazyVStack(alignment: .leading, spacing: 16) {
                    ForEach(Array(sessionVM.messages.enumerated()), id: \.offset) { _, message in
                        MessageView(message: message)
                    }
                    if sessionVM.isStreaming && !sessionVM.streamingText.isEmpty {
                        StreamingTextView(text: sessionVM.streamingText, thinking: sessionVM.streamingThinking)
                    }
                    if sessionVM.isStreaming && sessionVM.streamingText.isEmpty {
                        ThinkingIndicator()
                    }
                    if let error = sessionVM.errorMessage {
                        ErrorView(message: error)
                    }
                    Color.clear.frame(height: 1).id("bottom")
                }
                .padding(16)
            }
            .onChange(of: sessionVM.messages.count) { _ in
                withAnimation { proxy.scrollTo("bottom", anchor: .bottom) }
            }
            .onChange(of: sessionVM.streamingText) { _ in
                withAnimation { proxy.scrollTo("bottom", anchor: .bottom) }
            }
        }
    }
}

struct MessageView: View {
    let message: AgentMessage

    var body: some View {
        switch message {
        case .user(let content):
            UserMessageView(text: content)
        case .assistant(let id, let content, let stopReason):
            AssistantMessageView(content: content, stopReason: stopReason)
        case .toolResult(let results):
            ToolResultView(results: results)
        }
    }
}

struct UserMessageView: View {
    let text: String

    var body: some View {
        HStack {
            Spacer(minLength: 40)
            Text(text)
                .textSelection(.enabled)
                .padding(12)
                .background(Color.accentColor.opacity(0.12), in: RoundedRectangle(cornerRadius: 12, style: .continuous))
                .frame(maxWidth: 480, alignment: .trailing)
        }
    }
}

struct AssistantMessageView: View {
    let content: [AssistantMessageContent]
    let stopReason: String?

    var body: some View {
        VStack(alignment: .leading, spacing: 12) {
            ForEach(Array(content.enumerated()), id: \.offset) { _, part in
                switch part {
                case .text(let text):
                    Text(text)
                        .textSelection(.enabled)
                        .font(.body)
                case .thinking(let text):
                    Text(text)
                        .font(.callout)
                        .foregroundStyle(.secondary)
                        .italic()
                        .padding(8)
                        .background(Color.secondary.opacity(0.06), in: RoundedRectangle(cornerRadius: 8))
                case .toolCall(let call):
                    ToolCallView(call: call)
                }
            }
            if let stopReason, stopReason != "end_turn" {
                Text("Stopped: \(stopReason)")
                    .font(.caption)
                    .foregroundStyle(.tertiary)
            }
        }
        .frame(maxWidth: .infinity, alignment: .leading)
    }
}

struct ToolCallView: View {
    let call: ToolCall
    @State private var isExpanded = false

    var body: some View {
        VStack(alignment: .leading, spacing: 4) {
            Button {
                isExpanded.toggle()
            } label: {
                HStack(spacing: 6) {
                    Image(systemName: "wrench.and.screwdriver")
                        .font(.caption)
                    Text(call.name)
                        .font(.callout.monospaced())
                    Image(systemName: "chevron.right")
                        .font(.caption2)
                        .rotationEffect(.degrees(isExpanded ? 90 : 0))
                    Spacer()
                }
                .foregroundStyle(.secondary)
            }
            .buttonStyle(.plain)

            if isExpanded {
                Text(call.arguments.stringValue ?? String(describing: call.arguments))
                    .font(.caption.monospaced())
                    .textSelection(.enabled)
                    .padding(8)
                    .background(Color.secondary.opacity(0.08), in: RoundedRectangle(cornerRadius: 6))
            }
        }
    }
}

struct ToolResultView: View {
    let results: [ToolResult]

    var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            ForEach(Array(results.enumerated()), id: \.offset) { _, result in
                VStack(alignment: .leading, spacing: 4) {
                    if let name = result.toolName {
                        Text(name)
                            .font(.caption.weight(.semibold))
                            .foregroundStyle(result.isError == true ? .red : .secondary)
                    }
                    Text(result.content.stringValue ?? String(describing: result.content))
                        .font(.caption.monospaced())
                        .textSelection(.enabled)
                        .lineLimit(20)
                }
                .padding(8)
                .background(
                    (result.isError == true ? Color.red.opacity(0.08) : Color.secondary.opacity(0.06)),
                    in: RoundedRectangle(cornerRadius: 8)
                )
            }
        }
        .frame(maxWidth: .infinity, alignment: .leading)
    }
}

struct StreamingTextView: View {
    let text: String
    let thinking: String

    var body: some View {
        VStack(alignment: .leading, spacing: 12) {
            if !thinking.isEmpty {
                Text(thinking)
                    .font(.callout)
                    .foregroundStyle(.secondary)
                    .italic()
            }
            Text(text)
                .textSelection(.enabled)
                .font(.body)
        }
        .frame(maxWidth: .infinity, alignment: .leading)
    }
}

struct ThinkingIndicator: View {
    @State private var opacity: Double = 0.4

    var body: some View {
        HStack(spacing: 6) {
            Circle()
                .fill(Color.secondary)
                .frame(width: 6, height: 6)
                .opacity(opacity)
            Circle()
                .fill(Color.secondary)
                .frame(width: 6, height: 6)
                .opacity(opacity)
            Circle()
                .fill(Color.secondary)
                .frame(width: 6, height: 6)
                .opacity(opacity)
        }
        .onAppear {
            withAnimation(.easeInOut(duration: 0.6).repeatForever()) {
                opacity = 1.0
            }
        }
    }
}

struct ErrorView: View {
    let message: String

    var body: some View {
        HStack(spacing: 8) {
            Image(systemName: "exclamationmark.triangle.fill")
                .foregroundStyle(.red)
            Text(message)
                .font(.callout)
                .foregroundStyle(.red)
            Spacer()
        }
        .padding(10)
        .background(Color.red.opacity(0.08), in: RoundedRectangle(cornerRadius: 8))
    }
}

struct PermissionBanner: View {
    let perm: PermissionRequest
    let onApprove: () -> Void
    let onDeny: () -> Void

    var body: some View {
        HStack(spacing: 12) {
            Image(systemName: "shield.lefthalf.filled")
                .font(.title3)
                .foregroundStyle(.orange)
            VStack(alignment: .leading, spacing: 2) {
                Text("Permission Request: \(perm.toolName)")
                    .font(.headline)
                if let reason = perm.reason {
                    Text(reason)
                        .font(.caption)
                        .foregroundStyle(.secondary)
                }
            }
            Spacer()
            Button("Deny", role: .destructive, action: onDeny)
            Button("Allow", action: onApprove)
                .buttonStyle(.borderedProminent)
        }
        .padding(12)
        .background(Color.orange.opacity(0.08))
    }
}

struct QuestionBanner: View {
    let question: AskQuestionRequest
    let onAnswer: (String) -> Void
    @State private var selectedAnswer: String = ""

    var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            Text(question.question)
                .font(.headline)
            HStack(spacing: 8) {
                ForEach(question.options, id: \.self) { option in
                    Button(option) {
                        onAnswer(option)
                    }
                    .buttonStyle(.bordered)
                }
            }
        }
        .padding(12)
        .background(Color.blue.opacity(0.08))
    }
}
