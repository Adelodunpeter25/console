import { createFileRoute } from "@tanstack/react-router";
import { AssistantChat } from "../components/assistant-chat.js";

export const Route = createFileRoute("/")({
  component: IndexComponent,
});

function IndexComponent() {
  return <AssistantChat />;
}
