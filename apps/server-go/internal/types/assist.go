// Assist + usage wire types.
package types

type SlashCommandInfo struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Builtin     bool   `json:"builtin"`
}

// NotificationEvent moved to the shared protobuf schema
// (console.v1.NotificationEvent from proto/console/v1): the bus, builders,
// and SSE route all carry the generated type now.
