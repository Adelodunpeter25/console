import { create } from "zustand";
import { Actions, DockLocation, TabNode } from "flexlayout-react";
import { createWorkspaceModel } from "./model";
import {
  chatTabId,
  createChatTab,
  createFileTab,
  createTerminalTab,
  fileTabId,
  MAIN_WORKSPACE_TABSET_ID,
  terminalTabId,
} from "./types";
import type { OpenChatTabInput, OpenFileTabInput, OpenTerminalTabInput } from "./types";

interface WorkspaceState {
  model: ReturnType<typeof createWorkspaceModel>;
  revision: number;
  notifyLayoutChange: () => void;
  openChatTab: (input: OpenChatTabInput) => void;
  openFileTab: (input: OpenFileTabInput) => void;
  openTerminalTab: (input: OpenTerminalTabInput) => void;
  closeTerminalTab: (projectId: string, terminalId: string) => void;
  closeChatTab: (projectId: string, sessionId: string) => void;
  updateChatTabProject: (sessionId: string, projectId: string) => void;
}

export const useWorkspaceStore = create<WorkspaceState>((set, get) => ({
  model: createWorkspaceModel(),
  revision: 0,
  notifyLayoutChange: () => set((state) => ({ revision: state.revision + 1 })),

  openChatTab: (input) => {
    const { model } = get();
    const id = chatTabId(input.projectId, input.sessionId);
    const existing = model.getNodeById(id);

    if (existing instanceof TabNode) {
      model.doAction(Actions.selectTab(existing.getId()));
      set((state) => ({ revision: state.revision + 1 }));
      return;
    }

    model.doAction(
      Actions.addTab(createChatTab(input), MAIN_WORKSPACE_TABSET_ID, DockLocation.CENTER, -1, true),
    );
    set((state) => ({ model, revision: state.revision + 1 }));
  },

  openFileTab: (input) => {
    const { model } = get();
    const id = fileTabId(input.projectId, input.path);
    const existing = model.getNodeById(id);

    if (existing instanceof TabNode) {
      model.doAction(Actions.selectTab(existing.getId()));
      set((state) => ({ revision: state.revision + 1 }));
      return;
    }

    model.doAction(
      Actions.addTab(createFileTab(input), MAIN_WORKSPACE_TABSET_ID, DockLocation.CENTER, -1, true),
    );
    set((state) => ({ model, revision: state.revision + 1 }));
  },

  openTerminalTab: (input) => {
    const { model } = get();
    const id = terminalTabId(input.projectId, input.terminalId);
    const existing = model.getNodeById(id);

    if (existing instanceof TabNode) {
      model.doAction(Actions.selectTab(existing.getId()));
      set((state) => ({ revision: state.revision + 1 }));
      return;
    }

    model.doAction(
      Actions.addTab(createTerminalTab(input), MAIN_WORKSPACE_TABSET_ID, DockLocation.CENTER, -1, true),
    );
    set((state) => ({ model, revision: state.revision + 1 }));
  },

  closeTerminalTab: (projectId, terminalId) => {
    const { model } = get();
    const id = terminalTabId(projectId, terminalId);
    if (model.getNodeById(id)) {
      model.doAction(Actions.deleteTab(id));
      set((state) => ({ model, revision: state.revision + 1 }));
    }
  },

  closeChatTab: (projectId, sessionId) => {
    const { model } = get();
    const id = chatTabId(projectId, sessionId);
    if (model.getNodeById(id)) {
      model.doAction(Actions.deleteTab(id));
      set((state) => ({ model, revision: state.revision + 1 }));
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
      set((state) => ({ model, revision: state.revision + 1 }));
  },
}));
