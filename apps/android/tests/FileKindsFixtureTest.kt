package com.console.mobile

import com.console.mobile.data.model.isBinaryFileName
import com.console.mobile.data.model.isLockFileName
import com.console.mobile.data.model.isPreviewableImageName
import com.squareup.moshi.Moshi
import com.squareup.moshi.Types
import org.junit.Assert.assertTrue
import org.junit.Test
import java.io.File

/** Checks the Kotlin file-kind lists against proto/testdata/fs/file_kinds.json (shared with Go and Rust). */
class FileKindsFixtureTest {
    private val kinds: Map<String, List<String>> = Moshi.Builder().build()
        .adapter<Map<String, List<String>>>(
            Types.newParameterizedType(
                Map::class.java, String::class.java,
                Types.newParameterizedType(List::class.java, String::class.java)
            )
        )
        .fromJson(File("../../../proto/testdata/fs/file_kinds.json").readText())!!

    @Test
    fun lockFilesMatchFixture() {
        kinds.getValue("lock_basenames").forEach { assertTrue(it, isLockFileName(it)) }
        kinds.getValue("lock_suffixes").forEach { assertTrue(it, isLockFileName("x$it")) }
    }

    @Test
    fun rasterImagesMatchFixture() {
        kinds.getValue("raster_image_extensions").forEach {
            assertTrue(it, isPreviewableImageName("x$it"))
            assertTrue(it, isBinaryFileName("x$it"))
        }
    }

    @Test
    fun blockedExtensionsMatchFixture() {
        kinds.getValue("blocked_extensions").forEach { assertTrue(it, isBinaryFileName("x$it")) }
    }
}
