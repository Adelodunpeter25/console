/** Parse streamed function-call arguments without allowing malformed JSON to
 * abort the rest of the model turn. */
export function parseToolCallArguments(argumentsJson: string): unknown {
  try {
    return argumentsJson ? JSON.parse(argumentsJson) : {};
  } catch {
    return {};
  }
}

