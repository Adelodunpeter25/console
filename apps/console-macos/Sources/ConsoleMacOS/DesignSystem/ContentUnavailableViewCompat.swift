import SwiftUI

/// macOS 13-compatible replacement for ContentUnavailableView (macOS 14+).
/// Provides a simple centered empty-state with icon, title, and description.
struct ContentUnavailableViewCompat<Icon: View, Description: View>: View {
    let title: String
    let icon: Icon
    let description: Description

    init(_ title: String, @ViewBuilder icon: () -> Icon, @ViewBuilder description: () -> Description) {
        self.title = title
        self.icon = icon()
        self.description = description()
    }

    var body: some View {
        VStack(spacing: 12) {
            icon
                .font(.system(size: 32))
                .foregroundStyle(.secondary)
            Text(title)
                .font(.headline)
                .foregroundStyle(.secondary)
            description
                .font(.subheadline)
                .foregroundStyle(.tertiary)
                .multilineTextAlignment(.center)
        }
        .frame(maxWidth: .infinity, maxHeight: .infinity)
    }
}
