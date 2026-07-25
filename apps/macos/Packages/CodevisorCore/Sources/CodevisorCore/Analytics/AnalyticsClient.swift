import Foundation

public enum AnalyticsEventName: String, Sendable, CaseIterable {
    case appOpened = "app opened"
    case chatCreated = "chat created"
    case messageSent = "message sent"
    case modelSelected = "model selected"
    case harnessSelected = "harness selected"
    case turnCompleted = "turn completed"
    case turnFailed = "turn failed"
}

/// Analytics properties are deliberately scalar-only. There is no escape
/// hatch for arbitrary Encodable models, URLs, errors, or transcript objects,
/// which keeps prompts, responses, code, paths, and commands out by design.
public enum AnalyticsPropertyValue: Sendable, Equatable {
    case string(String)
    case integer(Int)
    case double(Double)
    case boolean(Bool)

}

/// No-op analytics client retained for source compatibility.
@MainActor
public final class AnalyticsClient {
    public static let shared = AnalyticsClient()

    private init() {}

    public func configureFromMainBundle(enabled: Bool) {}

    public func setEnabled(_ enabled: Bool) {}

    public func captureAppOpenedOnce() {}

    public func capture(
        _ event: AnalyticsEventName,
        properties: [String: AnalyticsPropertyValue] = [:]
    ) {}
}

public extension AnalyticsClient {
    /// Coarse token ranges preserve useful cost/scale segmentation without
    /// transmitting exact conversation usage.
    static func tokenBucket(_ value: UInt64?) -> String? {
        guard let value else { return nil }
        return switch value {
        case 0: "0"
        case 1..<1_000: "1-999"
        case 1_000..<10_000: "1k-9.9k"
        case 10_000..<100_000: "10k-99.9k"
        default: "100k+"
        }
    }
}
