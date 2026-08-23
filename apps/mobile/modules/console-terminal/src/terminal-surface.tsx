import { memo, useCallback, useEffect, useRef } from "react";
import {
  Pressable,
  ScrollView,
  Text,
  TextInput,
  View,
  type LayoutChangeEvent,
  type NativeSyntheticEvent,
  type ViewProps,
} from "react-native";

import {
  resolveNativeTerminalSurfaceView,
  type NativeTerminalSurfaceProps,
} from "./native-terminal-module";
import { buildGhosttyThemeConfig, CONSOLE_TERMINAL_THEME, type TerminalTheme } from "./terminal-theme";

interface TerminalInputEvent {
  readonly data: string;
}

interface TerminalResizeEvent {
  readonly cols: number;
  readonly rows: number;
}

export interface TerminalSurfaceProps extends ViewProps {
  readonly terminalKey: string;
  readonly buffer: string;
  readonly fontSize?: number;
  readonly isRunning: boolean;
  readonly autoFocus?: boolean;
  readonly keyboardFocusRequest?: number;
  readonly keyboardDismissRequest?: number;
  readonly theme?: TerminalTheme;
  readonly onInput: (data: string) => void;
  readonly onResize: (size: { readonly cols: number; readonly rows: number }) => void;
}

const DEFAULT_FONT_SIZE = 13;

function estimateGridSize(input: {
  readonly width: number;
  readonly height: number;
  readonly fontSize: number;
}): { readonly cols: number; readonly rows: number } {
  const cellWidth = input.fontSize * 0.62;
  const cellHeight = input.fontSize * 1.35;
  return {
    cols: Math.max(20, Math.min(400, Math.floor(input.width / cellWidth))),
    rows: Math.max(5, Math.min(200, Math.floor(input.height / cellHeight))),
  };
}

/** JS-only fallback used when the native surface isn't in the installed binary. */
const FallbackTerminalSurface = memo(function FallbackTerminalSurface(
  props: TerminalSurfaceProps & { readonly theme: TerminalTheme },
) {
  const fontSize = props.fontSize ?? DEFAULT_FONT_SIZE;
  const inputRef = useRef<TextInput>(null);
  const scrollRef = useRef<ScrollView>(null);
  const theme = props.theme;

  const handleLayout = (event: LayoutChangeEvent) => {
    const { width, height } = event.nativeEvent.layout;
    props.onResize(estimateGridSize({ width, height, fontSize }));
  };

  useEffect(() => {
    if ((props.keyboardFocusRequest ?? 0) > 0) {
      inputRef.current?.blur();
      const focusFrame = requestAnimationFrame(() => inputRef.current?.focus());
      return () => cancelAnimationFrame(focusFrame);
    }

    return undefined;
  }, [props.keyboardFocusRequest]);

  useEffect(() => {
    if ((props.keyboardDismissRequest ?? 0) > 0) {
      inputRef.current?.blur();
    }
  }, [props.keyboardDismissRequest]);

  return (
    <View
      className="flex-1"
      style={[
        {
          backgroundColor: theme.background,
          borderRadius: 8,
          overflow: "hidden",
        },
        props.style,
      ]}
      onLayout={handleLayout}
    >
      <View className="flex-1 px-2.5 py-2">
        <ScrollView
          ref={scrollRef}
          className="flex-1"
          showsVerticalScrollIndicator={false}
          onContentSizeChange={() => scrollRef.current?.scrollToEnd({ animated: false })}
          onScrollBeginDrag={() => inputRef.current?.blur()}
        >
          <Text
            selectable
            style={{
              color: theme.foreground,
              fontFamily: "JetBrainsMono",
              fontSize,
              lineHeight: Math.round(fontSize * 1.35),
            }}
          >
            {props.buffer || "$ "}
          </Text>
        </ScrollView>
      </View>
      <View
        className="flex-row items-center gap-2 border-t p-2"
        style={{
          borderTopColor: theme.border,
        }}
      >
        <TextInput
          ref={inputRef}
          autoCapitalize="none"
          autoCorrect={false}
          blurOnSubmit={false}
          editable={props.isRunning}
          placeholder="type and press return"
          placeholderTextColor={theme.mutedForeground}
          returnKeyType="send"
          className="text-sm"
          style={{
            color: theme.foreground,
            flex: 1,
            fontFamily: "JetBrainsMono",
            padding: 0,
          }}
          onSubmitEditing={(event) => {
            const text = event.nativeEvent.text;
            // blurOnSubmit is false, so clear after sending or the next return
            // keypress resends the same command.
            if (text.length > 0) {
              // Terminal Enter is CR. LF is Ctrl+J and raw-mode TUIs can treat it as J.
              props.onInput(`${text}\r`);
            }
            inputRef.current?.clear();
          }}
        />
        <Pressable
          disabled={!props.isRunning}
          style={({ pressed }) => ({
            opacity: !props.isRunning ? 0.35 : pressed ? 0.65 : 1,
            paddingHorizontal: 10,
            paddingVertical: 6,
            borderRadius: 8,
            backgroundColor: theme.border,
          })}
          onPress={() => props.onInput("\u0003")}
        >
          <Text className="text-2xs" style={{ color: theme.foreground }}>
            Ctrl-C
          </Text>
        </Pressable>
      </View>
    </View>
  );
});

export const ConsoleTerminalSurface = memo(function ConsoleTerminalSurface(
  props: TerminalSurfaceProps,
) {
  const fontSize = props.fontSize ?? DEFAULT_FONT_SIZE;
  const theme = props.theme ?? CONSOLE_TERMINAL_THEME;
  const { onInput, onResize } = props;
  const NativeSurfaceView = resolveNativeTerminalSurfaceView();

  const handleNativeInput = useCallback(
    (event: NativeSyntheticEvent<TerminalInputEvent>) => {
      if (!props.isRunning) {
        return;
      }
      onInput(event.nativeEvent.data);
    },
    [onInput, props.isRunning],
  );

  const handleNativeResize = useCallback(
    (event: NativeSyntheticEvent<TerminalResizeEvent>) => {
      onResize({
        cols: event.nativeEvent.cols,
        rows: event.nativeEvent.rows,
      });
    },
    [onResize],
  );

  if (NativeSurfaceView) {
    return (
      <View style={[{ flex: 1 }, props.style]}>
        <NativeSurfaceView
          appearanceScheme="dark"
          autoFocus={props.autoFocus ?? true}
          backgroundColor={theme.background}
          focusRequest={props.isRunning ? (props.keyboardFocusRequest ?? 0) : 0}
          dismissKeyboard={props.keyboardDismissRequest ?? 0}
          foregroundColor={theme.foreground}
          mutedForegroundColor={theme.mutedForeground}
          terminalKey={props.terminalKey}
          initialBuffer={props.buffer}
          fontSize={fontSize}
          style={{ flex: 1 }}
          themeConfig={buildGhosttyThemeConfig(theme)}
          onInput={handleNativeInput}
          onResize={handleNativeResize}
        />
      </View>
    );
  }

  return (
    <FallbackTerminalSurface
      {...props}
      fontSize={fontSize}
      theme={theme}
    />
  );
});

// Re-exported so app code only imports from this entry point.
export type { NativeTerminalSurfaceProps };
