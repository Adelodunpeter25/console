import { createFileRoute } from "@tanstack/react-router";
import { ProjectSidebar } from "../components/sidebar/project-sidebar.js";
import { AssistantChat } from "../components/chat/assistant-chat.js";

export const Route = createFileRoute("/")({
  component: IndexComponent,
});

function IndexComponent() {
  return <AssistantChat />;
}
