// Provider/model catalog types. Mirrors Model and ProviderCatalogEntry in
// packages/types/src/model.ts.
package types

type Model struct {
	ID              string   `json:"id"`
	Provider        string   `json:"provider"`
	ContextWindow   int      `json:"contextWindow"`
	SupportsImages  bool     `json:"supportsImages,omitempty"`
	ThinkingLevels  []string `json:"supportedThinkingLevels,omitempty"`
	DefaultThinking string   `json:"defaultThinkingLevel,omitempty"`
}

type ProviderEntry struct {
	Name        string  `json:"name"`
	DisplayName string  `json:"displayName"`
	Description string  `json:"description"`
	Models      []Model `json:"models"`
	AuthMethod  string  `json:"authMethod"`
}
