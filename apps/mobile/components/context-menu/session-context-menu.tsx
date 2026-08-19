import React, { useState, useRef, useCallback } from "react";
import { View } from "react-native";
import { Pencil, Trash2 } from "lucide-react-native";
import { BaseContextMenu, type ContextMenuItem } from "./base-context-menu";

interface SessionContextMenuProps {
  children: (onLongPress: () => void) => React.ReactNode;
  onRename: () => void;
  onDelete: () => void;
}

export function SessionContextMenu({
  children,
  onRename,
  onDelete,
}: SessionContextMenuProps) {
  const [visible, setVisible] = useState(false);
  const [anchor, setAnchor] = useState<{
    x: number;
    y: number;
    width: number;
    height: number;
  } | null>(null);
  const triggerRef = useRef<View>(null);

  const handleLongPress = useCallback(() => {
    triggerRef.current?.measure((_fx, _fy, width, height, px, py) => {
      setAnchor({ x: px, y: py, width, height });
      setVisible(true);
    });
  }, []);

  const items: ContextMenuItem[] = [
    {
      key: "rename",
      label: "Rename",
      icon: <Pencil size={16} color="#a1a1aa" />,
      onPress: onRename,
    },
    {
      key: "delete",
      label: "Delete",
      icon: <Trash2 size={16} color="#f87171" />,
      destructive: true,
      onPress: onDelete,
    },
  ];

  return (
    <>
      <View ref={triggerRef} collapsable={false}>
        {children(handleLongPress)}
      </View>

      <BaseContextMenu
        visible={visible}
        onClose={() => setVisible(false)}
        anchor={anchor}
        items={items}
      />
    </>
  );
}
