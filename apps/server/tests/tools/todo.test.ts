/**
 * Functional Tests for todoTool.
 * Zero network/API calls — 0 credits used.
 */
import assert from "node:assert/strict";
import { clearSessionTodoList, createTodoTool, todoTool } from "@/agent/src/tools/index.js";

console.log("Running TODO Tool tests...");

clearSessionTodoList();

// Session controllers must not share state or events.
{
  const eventsA: string[] = [];
  const controllerA = createTodoTool([], (items, action) => {
    eventsA.push(`${action}:${items.length}`);
  });
  const controllerB = createTodoTool();
  await controllerA.tool.execute(controllerA.tool.inputSchema.parse({ op: "init", tasks: ["A"] }));
  assert.equal(controllerA.getItems().length, 1);
  assert.equal(controllerB.getItems().length, 0);
  assert.deepEqual(eventsA, ["created:1"]);
  console.log("  ✅ todo session isolation and update event");
}

// 1. Init tasks
{
  const res = (await todoTool.execute(
    todoTool.inputSchema.parse({
      op: "init",
      tasks: ["Task 1: Setup database", "Task 2: Implement route"],
    }),
  )) as { content: Array<{ text: string }> };

  assert.ok(res.content[0]?.text.includes("Initialized task list"));
  assert.ok(res.content[0]?.text.includes("#1: Task 1: Setup database"));
  assert.ok(res.content[0]?.text.includes("#2: Task 2: Implement route"));
  console.log("  ✅ todo init operation");
}

// 2. Start task
{
  const res = (await todoTool.execute(
    todoTool.inputSchema.parse({
      op: "start",
      index: 1,
    }),
  )) as { content: Array<{ text: string }> };

  assert.ok(res.content[0]?.text.includes("Marked task #1 as in_progress"));
  assert.ok(res.content[0]?.text.includes("[>] #1: Task 1: Setup database (in_progress)"));
  console.log("  ✅ todo start operation");
}

// 3. Mark done
{
  const res = (await todoTool.execute(
    todoTool.inputSchema.parse({
      op: "done",
      index: 1,
    }),
  )) as { content: Array<{ text: string }> };

  assert.ok(res.content[0]?.text.includes("Marked task #1 as completed"));
  assert.ok(res.content[0]?.text.includes("[x] #1: Task 1: Setup database (completed)"));
  console.log("  ✅ todo done operation");
}

// 4. Append tasks
{
  const res = (await todoTool.execute(
    todoTool.inputSchema.parse({
      op: "append",
      tasks: ["Task 3: Add unit tests"],
    }),
  )) as { content: Array<{ text: string }> };

  assert.ok(res.content[0]?.text.includes("Appended tasks"));
  assert.ok(res.content[0]?.text.includes("#3: Task 3: Add unit tests"));
  console.log("  ✅ todo append operation");
}

// 5. View status
{
  const res = (await todoTool.execute(todoTool.inputSchema.parse({ op: "view" }))) as {
    content: Array<{ text: string }>;
  };

  assert.ok(res.content[0]?.text.includes("Current task list status:"));
  assert.ok(res.content[0]?.text.includes("#1: Task 1: Setup database"));
  assert.ok(res.content[0]?.text.includes("#2: Task 2: Implement route"));
  assert.ok(res.content[0]?.text.includes("#3: Task 3: Add unit tests"));
  console.log("  ✅ todo view operation");
}

clearSessionTodoList();
console.log("TODO Tool tests passed!\n");
