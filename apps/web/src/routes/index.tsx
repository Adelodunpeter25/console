import { createFileRoute } from "@tanstack/react-router";

export const Route = createFileRoute("/")({
  component: IndexComponent,
});

function IndexComponent() {
  return (
    <div className="p-6">
      <h1 className="text-xl font-semibold text-primary">Console</h1>
    </div>
  );
}
