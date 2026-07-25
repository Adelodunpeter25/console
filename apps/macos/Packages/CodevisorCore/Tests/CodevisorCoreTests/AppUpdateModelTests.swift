import Foundation
import Testing
@testable import CodevisorCore

@MainActor
@Suite("App update model")
struct AppUpdateModelTests {
    @Test("Update checks are no-ops")
    func checksAreNoOps() async {
        let model = AppUpdateModel(currentVersion: "1.2.3", currentBuildNumber: 42)

        await model.checkForUpdatesInBackground()
        await model.checkForUpdates()

        #expect(model.phase == .idle)
        #expect(model.currentBuildNumber == 42)
    }

    @Test("A discovered release can be installed")
    func availableReleaseInstalls() async throws {
        let releaseURL = URL(string: "https://updates.codevisor.dev/notes/1.2.4.md")
        let model = AppUpdateModel(currentVersion: "1.2.3")
        model.reportAvailable(version: "1.2.4", releasePageURL: releaseURL)
        let release = try #require(model.availableRelease)
        #expect(release.version == "1.2.4")
        #expect(release.releasePageURL == releaseURL)

        await model.installUpdate()
        #expect(model.phase == .available(release))

        model.reportInstalling(version: release.version, releasePageURL: release.releasePageURL)
        #expect(model.phase == .updating(release))
        #expect(model.isUpdating)
    }

    @Test("Failures retain the release for retry")
    func retryableFailure() {
        let model = AppUpdateModel(currentVersion: "1.2.3")
        model.reportAvailable(version: "1.2.4", releasePageURL: nil)
        model.reportFailure("network unavailable")

        #expect(
            model.phase == .failed(
                release: AppUpdateRelease(version: "1.2.4"),
                message: "network unavailable"
            )
        )
        #expect(model.failureMessage == "network unavailable")
        #expect(model.availableRelease?.version == "1.2.4")
    }

    @Test("Channel preference can still be changed")
    func channelChanges() {
        let model = AppUpdateModel(currentVersion: "1.2.3")

        model.setAllowsAlphaUpdates(true)
        #expect(model.allowsAlphaUpdates)
        model.setAllowsAlphaUpdates(false)

        #expect(!model.allowsAlphaUpdates)
    }

    @Test("Terminal report states are explicit")
    func terminalStates() {
        let model = AppUpdateModel(currentVersion: "1.2.3")
        model.reportUpToDate()
        #expect(model.phase == .upToDate)
        model.reportIdle()
        #expect(model.phase == .idle)
    }
}
