// Session-scoped TODO list item.
package types

type TodoItem struct {
	ID      int    `json:"id"`
	Content string `json:"content"`
	Status  string `json:"status"` // "pending" | "in_progress" | "completed"
}
