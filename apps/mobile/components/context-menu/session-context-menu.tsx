import React from "react";
import * as ContextMenu from "zeego/context-menu";

interface SessionContextMenuProps {
  children: React.ReactNode;
  onRename: () => void;
  onDelete: () => void;
}

export function SessionContextMenu({
  children,
  onRename,
  onDelete,
}: SessionContextMenuProps) {
  return (
    <ContextMenu.Root>
      <ContextMenu.Trigger>{children}</ContextMenu.Trigger>

      <ContextMenu.Content>
        <ContextMenu.Item key="rename" onSelect={onRename}>
          <ContextMenu.ItemTitle>Rename</ContextMenu.ItemTitle>
          <ContextMenu.ItemIcon
            ios={{ name: "pencil" }}
            androidIconName="ic_menu_edit"
          />
        </ContextMenu.Item>

        <ContextMenu.Item key="delete" destructive onSelect={onDelete}>
          <ContextMenu.ItemTitle>Delete</ContextMenu.ItemTitle>
          <ContextMenu.ItemIcon
            ios={{ name: "trash" }}
            androidIconName="ic_menu_delete"
          />
        </ContextMenu.Item>
      </ContextMenu.Content>
    </ContextMenu.Root>
  );
}
