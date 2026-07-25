import SwiftUI
import ConsoleCore

struct SettingsView: View {
    @ObservedObject var app: AppViewModel
    @Environment(\.dismiss) private var dismiss

    var body: some View {
        VStack(spacing: 0) {
            TabView {
                serverTab.tabItem { Label("Server", systemImage: "server.rack") }
                authTab.tabItem { Label("Auth", systemImage: "key.fill") }
                providersTab.tabItem { Label("Providers", systemImage: "cpu") }
            }
            .frame(width: 520, height: 380)

            Divider()

            HStack {
                Spacer()
                Button("Done") { dismiss() }
                    .keyboardShortcut(.defaultAction)
            }
            .padding(12)
        }
    }

    private var serverTab: some View {
        Form {
            Section("Backend URL") {
                TextField("Server URL", text: $app.serverURL, prompt: Text("http://localhost:3000"))
                    .textFieldStyle(.roundedBorder)

                Button("Test Connection") {
                    Task {
                        let ok = await app.pingServer()
                        if ok { await app.initialLoad() }
                    }
                }
            }

            Section("Status") {
                if app.isLoading {
                    HStack { ProgressView().controlSize(.small); Text("Loading…") }
                } else if let error = app.errorMessage {
                    Text(error).foregroundStyle(.red).font(.caption)
                } else {
                    Label("Connected", systemImage: "checkmark.circle.fill")
                        .foregroundStyle(.green)
                }
            }
        }
        .formStyle(.grouped)
        .padding()
    }

    private var authTab: some View {
        Form {
            if let status = app.authStatus {
                Section("Gemini") {
                    AuthStatusRow(loggedIn: status.gemini.loggedIn, email: status.gemini.email)
                    if !status.gemini.loggedIn {
                        Button("Login with Gemini") {
                            Task { await app.login(provider: "gemini") }
                        }
                    }
                }

                Section("Antigravity") {
                    AuthStatusRow(loggedIn: status.antigravity.loggedIn, email: status.antigravity.email)
                    if !status.antigravity.loggedIn {
                        Button("Login with Antigravity") {
                            Task { await app.login(provider: "antigravity") }
                        }
                    }
                }
            } else {
                ProgressView().frame(maxWidth: .infinity, alignment: .center)
            }
        }
        .formStyle(.grouped)
        .padding()
        .task {
            if app.authStatus == nil { await app.refreshAuthStatus() }
        }
    }

    private var providersTab: some View {
        Form {
            Section("Available Providers") {
                if app.providers.isEmpty {
                    Text("No providers loaded")
                        .foregroundStyle(.secondary)
                } else {
                    ForEach(app.providers) { provider in
                        VStack(alignment: .leading, spacing: 4) {
                            Text(provider.displayName)
                                .font(.headline)
                            Text(provider.description)
                                .font(.caption)
                                .foregroundStyle(.secondary)
                            HStack {
                                ForEach(provider.models) { model in
                                    Text(model.id)
                                        .font(.caption.monospaced())
                                        .padding(.horizontal, 6)
                                        .padding(.vertical, 2)
                                        .background(Color.secondary.opacity(0.1), in: Capsule())
                                }
                            }
                        }
                        .padding(.vertical, 4)
                    }
                }
            }
        }
        .formStyle(.grouped)
        .padding()
        .task {
            if app.providers.isEmpty { await app.refreshProviders() }
        }
    }
}

private struct AuthStatusRow: View {
    let loggedIn: Bool
    let email: String?

    var body: some View {
        HStack {
            Image(systemName: loggedIn ? "checkmark.circle.fill" : "xmark.circle")
                .foregroundStyle(loggedIn ? .green : .red)
            if let email {
                Text(email)
                    .font(.body)
            } else {
                Text(loggedIn ? "Logged in" : "Not logged in")
                    .foregroundStyle(.secondary)
            }
        }
    }
}
