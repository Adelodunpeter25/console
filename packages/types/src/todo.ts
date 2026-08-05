export type TodoStatus = "pending" | "in_progress" | "completed";

export interface TodoItem {
  id: number;
  content: string;
  status: TodoStatus;
}
