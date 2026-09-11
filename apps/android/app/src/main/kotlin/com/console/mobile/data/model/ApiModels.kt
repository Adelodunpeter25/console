package com.console.mobile.data.model

import kotlinx.serialization.Serializable
import kotlinx.serialization.json.JsonElement

@Serializable
data class ApiResponse<T>(
    val success: Boolean,
    val data: T? = null,
    val error: String? = null,
)

@Serializable
data class CreateSessionDto(
    val cwd: String? = null,
    val projectId: String? = null,
    val modelId: String? = null,
    val provider: String? = null,
    val title: String? = null,
    val approvalMode: String? = null,
)

@Serializable
data class UpdateSessionDto(
    val title: String? = null,
    val cwd: String? = null,
    val projectId: String? = null,
    val modelId: String? = null,
    val provider: String? = null,
    val approvalMode: String? = null,
)

@Serializable
data class RunPromptDto(
    val prompt: String,
    val modelId: String? = null,
    val provider: String? = null,
    val approvalMode: String? = null,
    val attachments: List<ImageAttachment> = emptyList(),
)

@Serializable
data class ImageAttachment(val data: String, val mimeType: String)

@Serializable
data class QueuedPrompt(
    val id: String,
    val sessionId: String,
    val prompt: String,
    val attachments: List<ImageAttachment> = emptyList(),
    val modelId: String? = null,
    val provider: String? = null,
    val approvalMode: String? = null,
    val createdAt: String,
)

@Serializable
data class OAuthLoginUrlDto(val provider: String)

@Serializable
data class OAuthCallbackDto(val provider: String, val code: String, val state: String? = null)

@Serializable
data class AnswerQuestionDto(val requestId: String, val answer: JsonElement)

@Serializable
data class ApproveToolPermissionDto(val requestId: String, val allow: Boolean)

@Serializable
data class SlashCommandInfo(val name: String, val description: String, val builtin: Boolean)

@Serializable
data class FileSearchResult(
    val relativePath: String,
    val absolutePath: String,
    val isDir: Boolean = false,
    val score: Double = 0.0,
)

@Serializable
data class FileSearchResponse(val root: String, val query: String, val items: List<FileSearchResult> = emptyList())

@Serializable
data class ProjectInfo(val id: String, val name: String, val path: String, val createdAt: Long, val updatedAt: Long)

@Serializable
data class ProviderAuthStatus(
    val loggedIn: Boolean,
    val email: String? = null,
    val projectId: String? = null,
    val configuredProjectId: String? = null,
)

@Serializable
data class SessionDetailResponse(
    val header: SessionHeader,
    val messages: List<AgentMessage> = emptyList(),
    val hasMore: Boolean = false,
    val nextCursor: Long? = null,
)

@Serializable
data class FsTreeEntry(
    val name: String,
    val path: String,
    val isDir: Boolean,
    val size: Long? = null,
    val gitStatus: String? = null,
    val children: List<FsTreeEntry>? = null,
)

@Serializable
data class GitFileEntry(
    val path: String,
    val status: String,
    val staged: Boolean = false,
    val additions: Int? = null,
    val deletions: Int? = null,
)

@Serializable
data class GitStatusSummary(val branch: String, val clean: Boolean, val files: List<GitFileEntry> = emptyList())

@Serializable
data class GitDiffResponse(val path: String? = null, val diff: String)

@Serializable
data class GitBranchInfo(val name: String, val current: Boolean)

@Serializable
data class GitBranchesResponse(val branches: List<GitBranchInfo> = emptyList(), val isGitRepository: Boolean)

@Serializable
data class FsChangeEvent(val type: String = "fsChange", val projectPath: String, val eventPath: String? = null)
