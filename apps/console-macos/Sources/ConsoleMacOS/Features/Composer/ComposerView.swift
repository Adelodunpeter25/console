import SwiftUI
import ConsoleCore

struct ComposerView: View {
    let isStreaming: Bool
    let onSend: (String, String?, String?) -> Void
    let onAbort: () -> Void
    let providers: [ProviderCatalogEntry]

    @State private var prompt = ""
    @State private var selectedModelId: String?
    @State private var selectedProvider: String?

    var body: some View {
        VStack(spacing: 8) {
            if !providers.isEmpty {
                HStack(spacing: 8) {
                    Picker("Model", selection: $selectedModelId) {
                        Text("Default").tag(nil as String?)
                        ForEach(providers) { provider in
                            ForEach(provider.models) { model in
                                Text("\(provider.displayName) — \(model.id)")
                                    .tag(model.id as String?)
                            }
                        }
                    }
                    .pickerStyle(.menu)
                    .frame(maxWidth: 280)

                    Picker("Provider", selection: $selectedProvider) {
                        Text("Default").tag(nil as String?)
                        ForEach(providers) { provider in
                            Text(provider.displayName).tag(provider.name as String?)
                        }
                    }
                    .pickerStyle(.menu)
                    .frame(maxWidth: 160)

                    Spacer()
                }
                .font(.caption)
            }

            HStack(alignment: .bottom, spacing: 8) {
                TextEditor(text: $prompt)
                    .font(.body)
                    .frame(minHeight: 36, maxHeight: 120)
                    .scrollContentBackground(.hidden)
                    .padding(8)
                    .background(Color(nsColor: .textBackgroundColor), in: RoundedRectangle(cornerRadius: 10))
                    .overlay(
                        RoundedRectangle(cornerRadius: 10)
                            .strokeBorder(Color(nsColor: .separatorColor), lineWidth: 1)
                    )

                if isStreaming {
                    Button {
                        onAbort()
                    } label: {
                        Image(systemName: "stop.fill")
                            .font(.title3)
                    }
                    .buttonStyle(.bordered)
                    .help("Stop")
                } else {
                    Button {
                        send()
                    } label: {
                        Image(systemName: "arrow.up.circle.fill")
                            .font(.title2)
                    }
                    .buttonStyle(.borderless)
                    .disabled(prompt.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty)
                    .help("Send")
                }
            }
        }
        .padding(12)
        .background(.bar)
    }

    private func send() {
        let trimmed = prompt.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !trimmed.isEmpty else { return }
        onSend(trimmed, selectedModelId, selectedProvider)
        prompt = ""
    }
}
