package com.console.mobile

import com.console.mobile.core.update.UpdateManifest
import com.console.mobile.core.update.UpdatePrompt
import com.console.mobile.core.update.isNewerThan
import com.console.mobile.core.update.parseUpdateManifest
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test
import java.io.File

class AppUpdateTest {
    @Test
    fun parsesPublishedManifestAndIgnoresExtraFields() {
        val manifest = parseUpdateManifest("""{"versionCode":42,"versionName":"1.0.1","sha":"abc"}""")
        assertEquals(UpdateManifest(42, "1.0.1"), manifest)
    }

    @Test
    fun onlyAStrictlyHigherBuildIsAnUpdate() {
        val manifest = UpdateManifest(10, "1.0.1")
        assertTrue(manifest.isNewerThan(9))
        assertFalse(manifest.isNewerThan(10))
        assertFalse(manifest.isNewerThan(11))
    }

    @Test
    fun updatePromptHoldsManifestAndApk() {
        val manifest = UpdateManifest(20, "1.0.5")
        val availablePrompt = UpdatePrompt.Available(manifest)
        assertEquals(20, availablePrompt.manifest.versionCode)

        val apkFile = File("/tmp/test.apk")
        val readyPrompt = UpdatePrompt.ReadyToInstall(manifest, apkFile)
        assertEquals(apkFile, readyPrompt.apk)
        assertEquals("1.0.5", readyPrompt.manifest.versionName)
    }
}
