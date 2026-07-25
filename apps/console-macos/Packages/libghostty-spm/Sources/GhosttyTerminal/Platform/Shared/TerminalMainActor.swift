import Foundation

@inline(__always)
func terminalRunOnMain(
    _ operation: @escaping @MainActor () -> Void
) {
    if Thread.isMainThread {
        operation()
        return
    }

    DispatchQueue.main.async {
        operation()
    }
}
