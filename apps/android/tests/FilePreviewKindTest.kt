package com.console.mobile

import com.console.mobile.data.model.FileDetails
import com.console.mobile.data.model.FilePreviewKind
import com.console.mobile.data.model.fileDetails
import com.console.mobile.data.model.filePreviewKind
import com.console.mobile.data.model.formatDurationMs
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test

/** The file viewer routes images to the raw-bytes preview, not the binary card. */
class FilePreviewKindTest {
    @Test
    fun rasterImagesPreview() {
        for (name in listOf("a.png", "b.JPG", "c.jpeg", "d.gif", "e.webp", "f.bmp", "g.ico")) {
            assertEquals(name, FilePreviewKind.IMAGE, filePreviewKind(name))
        }
    }

    @Test
    fun svgPreviews() {
        assertEquals(FilePreviewKind.IMAGE, filePreviewKind("icon.svg"))
    }

    @Test
    fun videoFilesGetPlayer() {
        for (name in listOf("clip.mp4", "clip.mov", "clip.AVI", "clip.mkv", "clip.webm")) {
            assertEquals(name, FilePreviewKind.VIDEO, filePreviewKind(name))
        }
    }

    @Test
    fun audioFilesGetPlayer() {
        for (name in listOf("song.mp3", "song.wav", "song.flac", "song.ogg", "song.m4a")) {
            assertEquals(name, FilePreviewKind.AUDIO, filePreviewKind(name))
        }
    }

    @Test
    fun pdfFilesGetViewer() {
        assertEquals(FilePreviewKind.PDF, filePreviewKind("doc.pdf"))
    }

    @Test
    fun opaqueBinariesGetDetailsCard() {
        for (name in listOf(
            "archive.zip", "font.ttf", "pkg.apk", "lib.so", "app.dmg", "data.db",
            "image.icns", "pic.tiff", "font.woff2", "lib.node", "pkg.deb", "disk.iso"
        )) {
            assertEquals(name, FilePreviewKind.DETAILS, filePreviewKind(name))
        }
    }

    @Test
    fun lockfilesStillBlock() {
        assertEquals(FilePreviewKind.BLOCKED, filePreviewKind("package-lock.json"))
        assertEquals(FilePreviewKind.BLOCKED, filePreviewKind("console.lock"))
    }

    @Test
    fun textFilesPreview() {
        for (name in listOf("main.kt", "readme.md", "notes.txt", "Makefile")) {
            assertEquals(name, FilePreviewKind.TEXT, filePreviewKind(name))
        }
    }
}

/** The details card skeleton: filename, MIME label, optional size/duration. */
class FileDetailsTest {
    @Test
    fun detailsCarryFilenameAndMime() {
        val details = fileDetails("archive.zip", sizeBytes = 2048)
        assertEquals("archive.zip", details.fileName)
        assertEquals("application/zip", details.mimeType)
        assertEquals(2048L, details.sizeBytes)
        assertNull(details.width)
        assertNull(details.height)
        assertNull(details.durationMs)
    }

    @Test
    fun mediaFilesGetMimeLabels() {
        assertEquals("video/mp4", fileDetails("clip.mp4").mimeType)
        assertEquals("audio/mpeg", fileDetails("song.mp3").mimeType)
        assertEquals("application/pdf", fileDetails("doc.pdf").mimeType)
    }

    @Test
    fun detailsFieldsFormat() {
        val details = FileDetails(
            fileName = "movie.mp4",
            mimeType = "video/mp4",
            sizeBytes = 12 * 1024 * 1024,
            width = 1920,
            height = 1080,
            durationMs = 75400,
        )
        val fields = details.fields.associate { it.first to it.second }
        assertEquals("video/mp4", fields["Type"])
        assertEquals("12.0 MB", fields["Size"])
        assertEquals("1920 × 1080", fields["Dimensions"])
        assertEquals("1:15", fields["Duration"])
    }

    @Test
    fun durationFormatsHours() {
        assertEquals("0:05", formatDurationMs(5000))
        assertEquals("1:15", formatDurationMs(75_000))
        assertEquals("1:02:03", formatDurationMs(3_723_000))
    }
}
