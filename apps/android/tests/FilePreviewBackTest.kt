package com.console.mobile

import com.console.mobile.feature.files.PreviewBack
import com.console.mobile.feature.files.previewBackAction
import org.junit.Assert.assertEquals
import org.junit.Test

/** System back from a file preview depends on how the file was opened. */
class FilePreviewBackTest {
    @Test
    fun linkOpenedFileGoesBackToThePreviousScreen() {
        assertEquals(PreviewBack.ToPreviousScreen, previewBackAction(openedFromLink = true))
    }

    @Test
    fun treeOpenedFileGoesBackToTheTree() {
        assertEquals(PreviewBack.ToFileTree, previewBackAction(openedFromLink = false))
    }
}
