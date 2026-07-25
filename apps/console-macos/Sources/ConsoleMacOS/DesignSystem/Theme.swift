import SwiftUI

/// Semantic color tokens read via `@Environment(\.theme)`.
/// Simplified from Codevisor — uses system colors only, no palette system.
struct Theme: Equatable {
    static let system = Theme()

    var windowBackground: Color { Color(nsColor: .windowBackgroundColor) }
    var sidebarBackground: Color { Color(nsColor: .controlBackgroundColor) }
    var cardBackground: Color { Color.secondary.opacity(0.08) }
    var composerBackground: Color { Color(nsColor: .controlBackgroundColor) }
    var bubbleBackground: Color { Color.primary.opacity(0.08) }

    var textPrimary: Color { Color.primary }
    var textSecondary: Color { Color.secondary }
    var textTertiary: Color { Color(nsColor: .tertiaryLabelColor) }

    var accent: Color { Color.accentColor }
    var border: Color { Color(nsColor: .separatorColor) }
    var separator: Color { Color(nsColor: .separatorColor) }

    var rowHoverBackground: Color { Color.secondary.opacity(0.12) }
    var rowSelectedBackground: Color { Color.primary.opacity(0.14) }

    var statusOK: Color { .green }
    var statusWarn: Color { .orange }
    var statusError: Color { .red }

    var diffAddedFg: Color { .green }
    var diffRemovedFg: Color { .red }
}

private struct ThemeKey: EnvironmentKey {
    static let defaultValue = Theme.system
}

extension EnvironmentValues {
    var theme: Theme {
        get { self[ThemeKey.self] }
        set { self[ThemeKey.self] = newValue }
    }
}

extension View {
    func themedRoot() -> some View {
        environment(\.theme, .system)
    }
}
