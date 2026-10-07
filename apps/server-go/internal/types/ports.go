// Port registry types.
package types

type ClientPort struct {
	Port      int     `json:"port"`
	URL       string  `json:"url"`
	ProjectID *string `json:"projectId,omitempty"`
}
