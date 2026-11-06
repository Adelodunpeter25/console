package com.console.mobile

import com.console.mobile.data.model.FilePreviewKind
import com.console.mobile.data.model.filePreviewKind
import org.junit.Assert.assertEquals
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
    fun nonImageBinariesStillBlock() {
        for (name in listOf("archive.zip", "movie.mp4", "font.ttf", "pkg.apk", "lib.so")) {
            assertEquals(name, FilePreviewKind.BLOCKED, filePreviewKind(name))
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
