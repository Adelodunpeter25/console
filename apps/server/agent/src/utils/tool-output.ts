/**
 * Normalize the MCP-style envelopes returned by built-in tools before they
 * enter the agent history or provider event stream.
 */
export function normalizeToolOutput(output: unknown): { content: unknown; isError?: boolean } {
  if (output && typeof output === "object" && !Array.isArray(output) && "content" in output) {
    const envelope = output as { content: unknown; isError?: unknown };
    return {
      content: envelope.content,
      ...(envelope.isError === true ? { isError: true } : {}),
    };
  }

  return { content: output };
}

