/**
 * Codebuff message converter.
 *
 * The Codebuff backend speaks the OpenAI chat-completions wire format, so we
 * reuse the OpenCode Zen converters (identical format) under a codebuff-named
 * alias.
 */
export { convertOpencodeMessages as convertCodebuffMessages } from "../opencode/convert-messages.js";