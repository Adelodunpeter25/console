import Foundation
import GhosttyTerminal

final class SessionBridge: @unchecked Sendable {
    nonisolated var session: InMemoryTerminalSession?
}
