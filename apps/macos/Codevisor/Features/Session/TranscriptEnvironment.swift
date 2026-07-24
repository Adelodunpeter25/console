import SwiftUI
import CodevisorCore

private struct TranscriptDisclosureKey: EnvironmentKey {
    static let defaultValue: TranscriptDisclosureStore? = nil
}

private struct RunningSubagentToolCallIdsKey: EnvironmentKey {
    static let defaultValue: Set<String> = []
}

private struct TranscriptControllerKey: EnvironmentKey {
    static let defaultValue: SessionController? = nil
}

private struct TranscriptPerformAnchoredDisclosureChangeKey: EnvironmentKey {
    static let defaultValue: ((@escaping () -> Void) -> Void)? = nil
}

private struct TranscriptInvalidateRowMeasurementKey: EnvironmentKey {
    static let defaultValue: (() -> Void)? = nil
}

extension EnvironmentValues {
    /// The session's disclosure store, injected at the transcript root. Nil in
    /// previews and detached contexts.
    var transcriptDisclosure: TranscriptDisclosureStore? {
        get { self[TranscriptDisclosureKey.self] }
        set { self[TranscriptDisclosureKey.self] = newValue }
    }

    /// Tool-call ids of subagents that are still running after their spawning
    /// turn ended.
    var runningSubagentToolCallIds: Set<String> {
        get { self[RunningSubagentToolCallIdsKey.self] }
        set { self[RunningSubagentToolCallIdsKey.self] = newValue }
    }

    /// Stable session facade used by deferred historical detail sections.
    var transcriptController: SessionController? {
        get { self[TranscriptControllerKey.self] }
        set { self[TranscriptControllerKey.self] = newValue }
    }

    /// Runs a user disclosure change while the containing transcript row is
    /// pinned to its current viewport position.
    var transcriptPerformAnchoredDisclosureChange: ((@escaping () -> Void) -> Void)? {
        get { self[TranscriptPerformAnchoredDisclosureChangeKey.self] }
        set { self[TranscriptPerformAnchoredDisclosureChangeKey.self] = newValue }
    }

    /// Requests a fresh intrinsic-height measurement from the containing
    /// native transcript row after isolated SwiftUI content changes.
    var transcriptInvalidateRowMeasurement: (() -> Void)? {
        get { self[TranscriptInvalidateRowMeasurementKey.self] }
        set { self[TranscriptInvalidateRowMeasurementKey.self] = newValue }
    }
}
