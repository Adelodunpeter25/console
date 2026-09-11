package com.console.mobile.core.util

import com.console.mobile.data.model.ToolCall
import com.console.mobile.data.model.ToolResult
import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.jsonPrimitive

val EXT_LANG_MAP: Map<String, String> = mapOf(
    "ts" to "typescript", "tsx" to "typescript", "js" to "javascript", "jsx" to "javascript",
    "mjs" to "javascript", "cjs" to "javascript", "json" to "json", "html" to "html",
    "css" to "css", "scss" to "scss", "py" to "python", "rb" to "ruby", "go" to "go",
    "rs" to "rust", "java" to "java", "kt" to "kotlin", "swift" to "swift",
    "c" to "c", "h" to "c", "cpp" to "cpp", "hpp" to "cpp", "cs" to "csharp",
    "php" to "php", "sh" to "bash", "bash" to "bash", "zsh" to "bash",
    "yml" to "yaml", "yaml" to "yaml", "toml" to "toml", "xml" to "xml",
    "md" to "markdown", "markdown" to "markdown", "sql" to "sql", "lua" to "lua",
    "r" to "r", "dart" to "dart", "vue" to "html", "svelte" to "html",
    "graphql" to "graphql", "dockerfile" to "dockerfile"
)

fun langFromPath(filePath: String): String? {
    val basename = filePath.split('/').lastOrNull().orEmpty()
    if (basename == "Dockerfile") return "dockerfile"
    if (basename == "Makefile") return "makefile"
    val ext = basename.substringAfterLast('.', "").lowercase()
    if (ext.isEmpty()) return null
    return EXT_LANG_MAP[ext]
}

val TOOL_LABELS: Map<String, String> = mapOf(
    "readFile" to "Read File", "writeFile" to "Write File", "batchWrite" to "Batch Write",
    "editFile" to "Edit File", "bash" to "Run Command", "bashJob" to "Bash Job",
    "grep" to "Search Code", "glob" to "Find Files", "listDir" to "List Directory",
    "fetch" to "Fetch URL", "webSearch" to "Web Search", "subagent" to "Subagent",
    "ask" to "Ask Question", "todo" to "Todo"
)

fun getToolLabel(name: String): String = TOOL_LABELS[name] ?: name

fun formatUnknown(v: Any?): String = when (v) {
    null -> "null"
    is String -> v
    else -> v.toString()
}

fun getFileName(filePath: String?): String {
    if (filePath.isNullOrEmpty()) return ""
    val clean = filePath.replace('\\', '/')
    return clean.split('/').lastOrNull().orEmpty().ifEmpty { filePath }
}

fun toRelativePath(filePath: String?, cwd: String?): String {
    if (filePath.isNullOrEmpty()) return ""
    val p = filePath.replace('\\', '/')
    val c = cwd?.replace('\\', '/')?.trimEnd('/')
    if (!c.isNullOrEmpty() && (p == c || p.startsWith("$c/"))) {
        return if (p == c) "." else p.substring(c.length + 1)
    }
    return p
}

fun toolCallSummary(call: ToolCall, cwd: String? = null): String? {
    val args = call.arguments as? JsonObject ?: return null
    fun s(key: String): String? = (args[key] as? JsonPrimitive)?.takeIf { it.isString }?.content
    if (call.name == "bashJob" && s("action") != null) {
        val job = s("jobId")
        return if (job != null) "${s("action")} $job" else s("action")
    }
    for (k in listOf("path", "filePath", "targetFile", "absolutePath")) {
        val v = s(k)
        if (v != null) return toRelativePath(v, cwd ?: s("cwd"))
    }
    val files = args["files"] as? JsonArray
    if (files != null && files.isNotEmpty()) {
        if (files.size == 1) {
            val p = ((files[0] as? JsonObject)?.get("path") as? JsonPrimitive)?.takeIf { it.isString }?.content
            if (p != null) return toRelativePath(p, cwd)
        }
        return "${files.size} files"
    }
    for (k in listOf("command", "query", "question")) {
        val v = s(k)
        if (v != null) return if (v.length > 45) v.take(42) + "…" else v
    }
    for (k in listOf("pattern", "url")) {
        val v = s(k)
        if (v != null) return v
    }
    val dir = s("directory")
    if (dir != null) return toRelativePath(dir, cwd)
    for (k in listOf("paths", "operations")) {
        val arr = args[k] as? JsonArray
        if (arr != null && arr.isNotEmpty()) return "${arr.size} ${if (k == "paths") "files" else "operations"}"
    }
    return null
}

fun resultText(result: ToolResult): String {
    val c = result.content ?: return "null"
    if (c is JsonPrimitive) return if (c.isString) c.content else c.toString()
    if (c is JsonObject) {
        val text = (c["text"] as? JsonPrimitive)?.takeIf { it.isString }?.content
        if (text != null) return text
        val inner = c["content"]
        if (inner != null && inner != c) return resultText(result.copy(content = inner))
        return c.toString()
    }
    if (c is JsonArray) {
        return c.joinToString("\n") { el ->
            when {
                el is JsonPrimitive && el.isString -> el.content
                el is JsonObject && (el["type"] as? JsonPrimitive)?.content == "text" ->
                    (el["text"] as? JsonPrimitive)?.takeIf { it.isString }?.content.orEmpty()
                else -> ""
            }
        }
    }
    return c.toString()
}
