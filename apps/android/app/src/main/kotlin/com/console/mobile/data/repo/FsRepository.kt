package com.console.mobile.data.repo

import com.console.mobile.data.api.ConsoleApi
import com.console.mobile.data.api.FsBrowseResult
import com.console.mobile.data.api.FsFileContent
import com.console.mobile.data.model.FileSearchResult
import com.console.mobile.data.model.FsTreeEntry
import com.console.mobile.data.store.FsStateHolder
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.withContext

class FsRepository(
    private val api: ConsoleApi,
    private val fsState: FsStateHolder,
    private val scope: CoroutineScope = CoroutineScope(SupervisorJob() + Dispatchers.Main.immediate),
) {
    suspend fun browseDirectory(path: String?): FsBrowseResult = withContext(Dispatchers.IO) {
        fsState.patch { it.copy(browsing = true, error = null) }
        try {
            val res = api.getFsBrowse(path)
            fsState.patch {
                it.copy(
                    browsePath = res.currentPath,
                    browseEntries = res.entries,
                    browsing = false,
                )
            }
            res
        } catch (e: Exception) {
            fsState.patch { it.copy(browsing = false, error = e.message ?: "Failed to browse") }
            throw e
        }
    }

    suspend fun getDirectoryTree(path: String?): List<FsTreeEntry> = withContext(Dispatchers.IO) {
        val res = api.getFsTree(path)
        val formatted = res.joinToString("\n") { formatTreeEntry(it) }
        val key = path ?: ""
        fsState.patch { it.copy(treesByPath = it.treesByPath + (key to formatted)) }
        res
    }

    suspend fun readFile(path: String): FsFileContent = withContext(Dispatchers.IO) {
        val cached = fsState.state.value.fileContentsByPath[path]
        if (cached != null) return@withContext FsFileContent(content = cached, path = path)

        val file = api.readFile(path)
        fsState.patch { it.copy(fileContentsByPath = it.fileContentsByPath + (path to file.content)) }
        file
    }

    suspend fun writeFile(path: String, content: String) = withContext(Dispatchers.IO) {
        fsState.markBusy(path)
        try {
            api.writeFile(path, content)
            fsState.invalidateFile(path)
        } finally {
            fsState.clearBusy(path)
        }
    }

    suspend fun deleteFile(path: String) = withContext(Dispatchers.IO) {
        fsState.markBusy(path)
        try {
            api.deleteFile(path)
            fsState.invalidateFile(path)
        } finally {
            fsState.clearBusy(path)
        }
    }

    suspend fun createDirectory(path: String) = withContext(Dispatchers.IO) {
        fsState.markBusy(path)
        try {
            api.createDir(path)
        } finally {
            fsState.clearBusy(path)
        }
    }

    suspend fun deleteDirectory(path: String) = withContext(Dispatchers.IO) {
        fsState.markBusy(path)
        try {
            api.deleteDir(path)
            fsState.patch { s ->
                s.copy(treesByPath = s.treesByPath.filterKeys { k -> k != path && !k.startsWith("$path/") })
            }
        } finally {
            fsState.clearBusy(path)
        }
    }

    suspend fun searchFiles(
        root: String,
        query: String,
        limit: Int = 20,
        includeDirs: Boolean = false,
    ): List<FileSearchResult> = withContext(Dispatchers.IO) {
        if (query.isBlank()) return@withContext emptyList()
        try {
            api.searchFiles(root, query.trim(), limit, includeDirs)
        } catch (_: Exception) {
            emptyList()
        }
    }

    private fun formatTreeEntry(entry: FsTreeEntry, depth: Int = 0): String {
        val indent = "  ".repeat(depth)
        val suffix = if (entry.isDir) "/" else ""
        return "$indent${entry.name}$suffix"
    }
}
