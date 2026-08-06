import { Model, TabNode } from "flexlayout-react";
import type { IJsonModel } from "flexlayout-react";
import { isWorkspaceTabConfig, MAIN_WORKSPACE_TABSET_ID } from "./types";
import type { WorkspaceTabConfig } from "./types";

export function createWorkspaceModel(): Model {
  const initialModel: IJsonModel = {
    global: {
      tabEnableDrag: true,
      tabEnableRename: false,
      tabSetEnableDivide: true,
      tabSetEnableMaximize: true,
      tabSetEnableSingleTabStretch: false,
    },
    layout: {
      type: "row",
      weight: 100,
      children: [
        {
          type: "tabset",
          id: MAIN_WORKSPACE_TABSET_ID,
          weight: 100,
          enableDeleteWhenEmpty: false,
          children: [],
        },
      ],
    },
  };

  return Model.fromJson(initialModel);
}

/** Returns the config for the tab currently active in the workspace. */
export function getActiveWorkspaceTab(model: Model): WorkspaceTabConfig | null {
  const tab = model.getActiveTabset()?.getSelectedNode();
  if (!(tab instanceof TabNode)) return null;

  const config = tab.getConfig();
  return isWorkspaceTabConfig(config) ? config : null;
}
