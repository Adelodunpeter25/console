// Provider/model catalog types (Model and ProviderCatalogEntry).
//
// These are the server-internal catalog definitions, NOT the wire shape:
// routes/providers.go converts them to console.v1.Model /
// console.v1.ProviderCatalogEntry (proto/console/v1/catalog.proto), which
// is what clients decode. Thinking levels stay plain strings here and on
// the wire; the client-side ThinkingLevel enum is a separate concern.
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