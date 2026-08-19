import React, { useState, useRef, useCallback } from "react";
import { View } from "react-native";
import * as Clipboard from "expo-clipboard";
import { Copy, FolderOpen } from "lucide-react-native";
import { BaseContextMenu, type ContextMenuItem } from "./base-context-menu";

interface FileContextMenuProps {
  /** Absolute path to the file or folder */
  path: string;
  /** Path relative to the project root, shown as "Copy Relative Path" */
  relativePath?: string;
  children: (onLongPress: () => void) => React.ReactNode;
}

export function FileContextMenu({ path, relativePath, children }: FileContextMenuProps) {
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
      key: "copy-path",
      label: "Copy Path",
      icon: <Copy size={15} color="#a1a1aa" />,
      onPress: () => Clipboard.setStringAsync(path),
    },
    ...(relativePath
      ? [
          {
            key: "copy-relative-path",
            label: "Copy Relative Path",
            icon: <FolderOpen size={15} color="#a1a1aa" />,
            onPress: () => Clipboard.setStringAsync(relativePath),
          },
        ]
      : []),
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
