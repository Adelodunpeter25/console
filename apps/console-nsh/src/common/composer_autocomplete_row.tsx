// One row of the composer autocomplete popup, ported from console-ui's
// `autocomplete.rs` popup: the item's kind glyph, its label, and the muted
// detail (a command's description, a file's directory, a chat's title).
import { Text, View, token } from "nsh";
import { Icon, type IconName } from "./icons";
import { fileTypeIconName } from "../chat/change_chips";
import type { AutocompleteItem } from "./composer_autocomplete";

export function ComposerAutocompleteRow(props: {
  item: AutocompleteItem;
  selected: boolean;
  onConfirm: (item: AutocompleteItem) => void;
}) {
  const { item } = props;
  const kindIcon = (): IconName => {
    if (item.kind === "file") return fileTypeIconName(item.file.relativePath) as IconName;
    if (item.kind === "chat") return "ChatRoundLine";
    return "Slash";
  };
  const label = (): string => {
    if (item.kind === "command") return `/${item.command.name}`;
    if (item.kind === "file") {
      const path = item.file.relativePath || item.file.absolutePath;
      return path.split("/").pop() ?? path;
    }
    return item.chat.title;
  };
  const detail = (): string | null => {
    if (item.kind === "command") return item.command.description;
    if (item.kind === "file") {
      const path = item.file.relativePath || item.file.absolutePath;
      const at = path.lastIndexOf("/");
      return at >= 0 ? path.slice(0, at + 1) : null;
    }
    return null;
  };
  return (
    <View
      class={
        "flex-row items-center gap-2 rounded-md px-2 py-1.5 " +
        (props.selected ? "bg-accent" : "")
      }
      role="button"
      accessibilityLabel={`Autocomplete ${label()}`}
      onPress={() => props.onConfirm(item)}
    >
      <Icon name={kindIcon()} size={13} color={token("muted-foreground")} />
      <Text class="text-[12px] font-medium text-foreground">{label()}</Text>
      {detail() ? (
        <View class="min-w-0 flex-1 overflow-hidden">
          <Text class="text-[11px] text-muted-foreground">{detail()}</Text>
        </View>
      ) : (
        <View class="flex-1" />
      )}
      {props.selected ? (
        <Text class="flex-none text-[10px] text-muted-foreground">
          Enter ↵
        </Text>
      ) : null}
    </View>
  );
}
