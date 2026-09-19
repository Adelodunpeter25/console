// Port registry types ported from port-registry.service.ts.
package types

type ClientPort struct {
	Port      int     `json:"port"`
	URL       string  `json:"url"`
	ProjectID *string `json:"projectId,omitempty"`
}
