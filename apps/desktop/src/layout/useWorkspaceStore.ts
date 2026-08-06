import { create } from "zustand";
import { Actions, DockLocation, TabNode } from "flexlayout-react";
import { createWorkspaceModel } from "./model";
import { chatTabId, createChatTab, MAIN_WORKSPACE_TABSET_ID } from "./types";
import type { OpenChatTabInput } from "./types";

interface WorkspaceState {
  model: ReturnType<typeof createWorkspaceModel>;
  openChatTab: (input: OpenChatTabInput) => void;
  closeChatTab: (projectId: string, sessionId: string) => void;
  updateChatTabProject: (sessionId: string, projectId: string) => void;
}

export const useWorkspaceStore = create<WorkspaceState>((set, get) => ({
  model: createWorkspaceModel(),

  openChatTab: (input) => {
    const { model } = get();
    const id = chatTabId(input.projectId, input.sessionId);
    const existing = model.getNodeById(id);

    if (existing instanceof TabNode) {
      model.doAction(Actions.selectTab(existing.getId()));
      return;
    }

    model.doAction(
      Actions.addTab(createChatTab(input), MAIN_WORKSPACE_TABSET_ID, DockLocation.CENTER, -1, true),
    );
    // Keep the model reference stable while notifying subscribers that a
    // layout action occurred. FlexLayout itself listens to the model too.
    set({ model });
  },

  closeChatTab: (projectId, sessionId) => {
    const { model } = get();
    const id = chatTabId(projectId, sessionId);
    if (model.getNodeById(id)) {
      model.doAction(Actions.deleteTab(id));
      set({ model });
    }
  },

  updateChatTabProject: (sessionId, projectId) => {
    const { model } = get();
    const id = chatTabId("", sessionId);
    const tab = model.getNodeById(id);
    if (!(tab instanceof TabNode)) return;

    const config = tab.getConfig();
    if (!config || config.type !== "chat") return;
    model.doAction(
      Actions.updateNodeAttributes(tab.getId(), {
        config: { ...config, projectId },
      }),
    );
    set({ model });
  },
}));
