import SwiftUI
import CodeEditor
import ConsoleCore

/// A code viewer/editor for files fetched from the backend.
/// Uses the CodeEditor package (https://github.com/ZeeZide/CodeEditor)
/// for syntax highlighting via Highlight.js.
struct CodeEditorView: View {
    @ObservedObject var app: AppViewModel
    let path: String

    @State private var source: String = ""
    @State private var isLoading = false
    @State private var error: String?
    @State private var isEditing = false
    @State private var hasUnsavedChanges = false
    @State private var language: CodeEditor.Language = .swift

    var body: some View {
        VStack(spacing: 0) {
            toolbar
            Divider()
            if isLoading {
                ProgressView("Loading \(fileName)…")
                    .frame(maxWidth: .infinity, maxHeight: .infinity)
            } else if let error {
                VStack(spacing: 12) {
                    Image(systemName: "exclamationmark.triangle")
                        .font(.largeTitle)
                        .foregroundStyle(.red)
                    Text(error)
                        .foregroundStyle(.secondary)
                    Button("Retry") { Task { await loadFile() } }
                }
                .frame(maxWidth: .infinity, maxHeight: .infinity)
            } else {
                CodeEditor(
                    source: $source,
                    language: language,
                    theme: .ocean,
                    fontSize: nil,
                    flags: isEditing ? .defaultEditorFlags : .defaultViewerFlags,
                    indentStyle: .softTab(width: 4),
                    allowsUndo: isEditing
                )
                .onChange(of: source) { _ in
                    hasUnsavedChanges = true
                }
            }
        }
        .task { await loadFile() }
    }

    private var fileName: String {
        (path as NSString).lastPathComponent
    }

    private var toolbar: some View {
        HStack(spacing: 12) {
            Text(fileName)
                .font(.headline)
                .lineLimit(1)

            Picker("Language", selection: $language) {
                ForEach(CodeEditor.availableLanguages) { lang in
                    Text(lang.rawValue.capitalized).tag(lang)
                }
            }
            .pickerStyle(.menu)
            .frame(maxWidth: 160)
            .font(.caption)

            Spacer()

            if hasUnsavedChanges {
                Text("Unsaved")
                    .font(.caption)
                    .foregroundStyle(.orange)
            }

            Toggle("Edit", isOn: $isEditing)
                .toggleStyle(.switch)
                .controlSize(.small)

            if isEditing && hasUnsavedChanges {
                Button("Save") {
                    Task { await saveFile() }
                }
                .buttonStyle(.borderedProminent)
            }
        }
        .padding(.horizontal, 12)
        .padding(.vertical, 8)
    }

    private func loadFile() async {
        isLoading = true
        error = nil
        do {
            let result = try await app.client.readFile(path: path)
            if let text = result.stringValue {
                source = text
            } else if let data = try? JSONSerialization.data(withJSONObject: result),
                      let str = String(data: data, encoding: .utf8) {
                source = str
            }
            detectLanguage()
        } catch {
            self.error = error.localizedDescription
        }
        isLoading = false
    }

    private func saveFile() async {
        do {
            _ = try await app.client.writeFile(path: path, content: source)
            hasUnsavedChanges = false
        } catch {
            self.error = error.localizedDescription
        }
    }

    private func detectLanguage() {
        let ext = (path as NSString).pathExtension.lowercased()
        switch ext {
        case "swift": language = .swift
        case "rs": language = .rust
        case "ts": language = .typescript
        case "js": language = .javascript
        case "json": language = .json
        case "py": language = .python
        case "rb": language = .ruby
        case "go": language = .go
        case "css": language = .css
        case "html": language = .xml
        case "xml": language = .xml
        case "yaml", "yml": language = .yaml
        case "md": language = .markdown
        case "sh": language = .shell
        case "sql": language = .sql
        case "diff", "patch": language = .diff
        case "dockerfile": language = .dockerfile
        default: language = CodeEditor.Language(rawValue: ext) ?? .swift
        }
    }
}
