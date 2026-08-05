/**
 * Represents the configuration and capabilities of an AI model.
 */
export interface Model {
  /**
   * A unique identifier for the model (e.g., "gemini-1.5-pro-latest").
   */
  id: string;
  /**
   * The provider of the model (e.g., "gemini", "antigravity").
   */
  provider: "gemini" | "antigravity";
  /**
   * The maximum number of tokens (input + output) the model can handle.
   */
  contextWindow: number;
  /** Whether the provider explicitly reports support for image input. */
  supportsImages?: boolean;
}
