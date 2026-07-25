import Combine
import Foundation

/// Framework-independent app update state. Update operations are disabled.
public struct AppUpdateRelease: Equatable, Sendable {
    public var version: String
    public var releasePageURL: URL?

    public init(version: String, releasePageURL: URL? = nil) {
        self.version = version
        self.releasePageURL = releasePageURL
    }
}

@MainActor
public final class AppUpdateModel: ObservableObject {
    public enum Phase: Equatable, Sendable {
        case idle
        case checking
        case upToDate
        case available(AppUpdateRelease)
        case updating(AppUpdateRelease)
        case failed(release: AppUpdateRelease?, message: String)
    }

    @Published public private(set) var phase: Phase = .idle
    public let currentVersion: String
    public let currentBuildNumber: Int?
    @Published public private(set) var allowsAlphaUpdates: Bool

    public init(
        currentVersion: String,
        currentBuildNumber: Int? = nil,
        allowsAlphaUpdates: Bool = false
    ) {
        self.currentVersion = currentVersion
        self.currentBuildNumber = currentBuildNumber
        self.allowsAlphaUpdates = allowsAlphaUpdates
    }

    public func setAllowsAlphaUpdates(_ value: Bool) {
        allowsAlphaUpdates = value
    }

    public var availableRelease: AppUpdateRelease? {
        switch phase {
        case let .available(release), let .updating(release):
            return release
        case let .failed(release, _):
            return release
        case .idle, .checking, .upToDate:
            return nil
        }
    }

    public var isUpdating: Bool {
        if case .updating = phase { return true }
        return false
    }

    public var failureMessage: String? {
        if case let .failed(_, message) = phase { return message }
        return nil
    }

    public func checkForUpdates() async {}

    public func checkForUpdatesInBackground() async {}

    public func installUpdate() async {}

    public func installUpdateUnattended() async {}

    public func reportAvailable(version: String, releasePageURL: URL?) {
        let release = AppUpdateRelease(version: version, releasePageURL: releasePageURL)
        if case let .failed(existing, message) = phase, existing?.version == version {
            phase = .failed(release: release, message: message)
        } else {
            phase = .available(release)
        }
    }

    public func reportUpToDate() {
        phase = .upToDate
    }

    public func reportInstalling(version: String, releasePageURL: URL?) {
        phase = .updating(
            AppUpdateRelease(version: version, releasePageURL: releasePageURL)
        )
    }

    public func reportFailure(_ message: String) {
        phase = .failed(release: availableRelease, message: message)
    }

    public func reportIdle() {
        phase = .idle
    }

    public static func bundleVersion(_ bundle: Bundle = .main) -> String {
        (bundle.object(forInfoDictionaryKey: "CFBundleShortVersionString") as? String) ?? "0.0.0"
    }

    public static func bundleBuildNumber(_ bundle: Bundle = .main) -> Int? {
        guard let value = bundle.object(forInfoDictionaryKey: "CFBundleVersion") as? String else {
            return nil
        }
        return Int(value)
    }

    public static func bundleSourceRevision(_ bundle: Bundle = .main) -> String? {
        guard let value = bundle.object(forInfoDictionaryKey: "CodevisorSourceRevision") as? String,
              !value.isEmpty, value != "unknown"
        else { return nil }
        return value
    }
}
