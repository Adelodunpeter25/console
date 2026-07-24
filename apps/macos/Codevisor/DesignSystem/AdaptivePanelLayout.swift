import SwiftUI

enum AdaptiveDrawer: Equatable {
    case leading
    case trailing
}

/// Window-scoped presentation state for the navigation sidebar and session
/// inspector. Docking is derived from window width; the drawer is transient,
/// so responsive changes never overwrite the user's saved visibility choices.
@MainActor
final class AdaptivePanelLayout: ObservableObject {
    private static let inspectorCollapseWidth: CGFloat = 960
    private static let inspectorRestoreWidth: CGFloat = 1_000
    private static let sidebarCollapseWidth: CGFloat = 720
    private static let sidebarRestoreWidth: CGFloat = 760

    @Published private(set) var windowWidth: CGFloat = 1_280
    @Published private(set) var docksSidebar = true
    @Published private(set) var docksInspector = true
    @Published private(set) var activeDrawer: AdaptiveDrawer?

    func updateWindowWidth(_ width: CGFloat) {
        guard width.isFinite, width > 0, abs(width - windowWidth) > 0.5 else { return }
        windowWidth = width

        if docksInspector, width < Self.inspectorCollapseWidth {
            docksInspector = false
        } else if !docksInspector, width > Self.inspectorRestoreWidth {
            docksInspector = true
            if activeDrawer == .trailing { activeDrawer = nil }
        }

        if docksSidebar, width < Self.sidebarCollapseWidth {
            docksSidebar = false
        } else if !docksSidebar, width > Self.sidebarRestoreWidth {
            docksSidebar = true
            if activeDrawer == .leading { activeDrawer = nil }
        }
    }

    func toggleDrawer(_ drawer: AdaptiveDrawer) {
        activeDrawer = activeDrawer == drawer ? nil : drawer
    }

    func dismissDrawer(_ drawer: AdaptiveDrawer? = nil) {
        guard drawer == nil || activeDrawer == drawer else { return }
        activeDrawer = nil
    }
}

/// A floating, edge-aligned panel above the primary content. Keeping this as
/// an overlay instead of another split-view column protects the chat's width
/// in compact windows.
struct AdaptiveDrawerLayer<DrawerContent: View>: View {
    @EnvironmentObject private var panelLayout: AdaptivePanelLayout
    @Environment(\.accessibilityReduceMotion) private var reduceMotion

    let isPresented: Bool
    let edge: Edge
    let width: CGFloat
    @ViewBuilder var drawerContent: () -> DrawerContent

    var body: some View {
        ZStack(alignment: edge == .leading ? .leading : .trailing) {
            if isPresented {
                Color.black.opacity(0.12)
                    .contentShape(Rectangle())
                    .onTapGesture { panelLayout.dismissDrawer() }
                    .transition(.opacity)

                drawerContent()
                    .frame(width: width)
                    .padding(8)
                    .transition(.move(edge: edge).combined(with: .opacity))
            }
        }
        .allowsHitTesting(isPresented)
        .onExitCommand { panelLayout.dismissDrawer() }
        .animation(Motion.quick(reduceMotion: reduceMotion), value: isPresented)
    }
}
