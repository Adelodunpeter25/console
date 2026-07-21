/**
 * Parses a text/event-stream (SSE) response body into JSON objects.
 * Each `data: {...}` line is parsed and yielded.
 */
export async function* parseSse<T>(response: Response): AsyncGenerator<T> {
  if (!response.body) {
    throw new Error("No response body from CCA endpoint");
  }

  const decoder = new TextDecoder();
  let buffer = "";

  for await (const chunk of response.body as unknown as AsyncIterable<Uint8Array>) {
    buffer += decoder.decode(chunk, { stream: true });

    const lines = buffer.split("\n");
    // Keep the last (potentially incomplete) line in the buffer
    buffer = lines.pop() ?? "";

    for (const line of lines) {
      const trimmed = line.trim();
      if (!trimmed.startsWith("data:")) continue;

      const jsonText = trimmed.slice(5).trim();
      if (jsonText === "[DONE]" || jsonText === "") continue;

      try {
        yield JSON.parse(jsonText) as T;
      } catch {
        // malformed SSE chunk — skip
      }
    }
  }

  // Flush remaining buffer
  if (buffer.trim().startsWith("data:")) {
    const jsonText = buffer.trim().slice(5).trim();
    if (jsonText && jsonText !== "[DONE]") {
      try {
        yield JSON.parse(jsonText) as T;
      } catch {
        // ignore
      }
    }
  }
}
