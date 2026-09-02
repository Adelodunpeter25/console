import React, { useState, useRef } from "react";
import { View, TextInput } from "react-native";
import { useSafeAreaInsets } from "react-native-safe-area-context";
import { KeyboardStickyView, useKeyboardState } from "react-native-keyboard-controller";
import * as ImagePicker from "expo-image-picker";
import type { ImageAttachment } from "@console/types";
import { addAttachments, getChatSession, removeAttachment } from "@/stores/useChatStore";
import { changeModel, changeProject, setApprovalMode, sessionsView$ } from "@/stores/useSessionStore";
import { ComposerBottomStrip } from "./composer-bottom-strip";
import { ComposerAutocomplete } from "./composer-autocomplete";
import { AttachmentStrip } from "./attachment-strip";
import { ComposerInput } from "./composer-input";
import { app$ } from "@/stores/useAppStore";
import { useValue } from "@legendapp/state/react";
import { project$ } from "@/stores/useProjectStore";

interface ComposerProps {
  value: string;
  onChangeText: (text: string) => void;
  onSend: () => void;
  onStop?: () => void;
  running?: boolean;
  isAborting?: boolean;
  projectLocked?: boolean;
  topBanner?: React.ReactNode;
}

export function Composer({
  value,
  onChangeText,
  onSend,
  onStop,
  running,
  isAborting,
  projectLocked,
  topBanner,
}: ComposerProps) {
  const insets = useSafeAreaInsets();
  const keyboardVisible = useKeyboardState((s) => s.isVisible);
  const paddingBottom = keyboardVisible ? 8 : Math.max(insets.bottom, 8) + 4;

  const selectedSessionId = useValue(app$.selectedSessionId);
  const projects = useValue(project$.projects);
  const sessionView = useValue(() =>
    selectedSessionId ? sessionsView$[selectedSessionId].get() : undefined,
  );

  const attachments = useValue(() =>
    selectedSessionId ? getChatSession(selectedSessionId).attachments : [],
  );

  const selectedProject = projects.find(
    (p) =>
      p.path === sessionView?.sessionCwd ||
      (sessionView?.sessionCwd && p.path.endsWith(sessionView.sessionCwd)),
  );

  const [selection, setSelection] = useState<{ start: number; end: number }>({
    start: value.length,
    end: value.length,
  });
  const inputRef = useRef<TextInput>(null);
  const canSend = value.trim().length > 0 || attachments.length > 0;

  const handlePickImages = async () => {
    if (!selectedSessionId) return;
    try {
      if (!ImagePicker?.launchImageLibraryAsync) {
        console.warn("ImagePicker native module not found in binary");
        return;
      }
      const result = await ImagePicker.launchImageLibraryAsync({
        mediaTypes: ["images"],
        allowsMultipleSelection: true,
        base64: true,
        quality: 0.85,
      });

      if (!result.canceled && result.assets && result.assets.length > 0) {
        const newAttachments: ImageAttachment[] = result.assets
          .filter((asset) => Boolean(asset.base64))
          .map((asset) => ({
            data: asset.base64!,
            mimeType: asset.mimeType ?? "image/jpeg",
          }));

        if (newAttachments.length > 0) {
          addAttachments(selectedSessionId, newAttachments);
        }
      }
    } catch (error) {
      console.warn("Failed to pick images from file explorer:", error);
    }
  };

  const handleAutocompletePick = (newValue: string) => {
    onChangeText(newValue);
    const newPos = newValue.length;
    setSelection({ start: newPos, end: newPos });
    setTimeout(() => inputRef.current?.focus(), 50);
  };

  return (
    <>
      <KeyboardStickyView offset={{ closed: 0, opened: 0 }}>
        <View className="px-2.5 pt-2 bg-screen" style={{ paddingBottom }}>
          {topBanner}

          <ComposerAutocomplete
            value={value}
            selectionStart={selection.start}
            sessionId={selectedSessionId}
            onPick={handleAutocompletePick}
          />

          <AttachmentStrip
            attachments={attachments}
            onRemove={(idx) => removeAttachment(selectedSessionId!, idx)}
          />

          <ComposerInput
            value={value}
            onChangeText={onChangeText}
            selection={selection}
            onSelectionChange={setSelection}
            inputRef={inputRef}
            canSend={canSend}
            running={running}
            onSend={onSend}
            onStop={onStop}
            onPickImage={handlePickImages}
          />

          {selectedSessionId ? (
            <ComposerBottomStrip
              projects={projects}
              selectedProjectId={selectedProject?.id ?? null}
              onProjectChange={(project) => changeProject(selectedSessionId, project)}
              projectLocked={projectLocked}
              selectedModel={sessionView?.sessionModelId ?? null}
              selectedProvider={sessionView?.sessionProvider ?? null}
              onModelChange={(modelId) => changeModel(selectedSessionId, modelId)}
              approvalMode={sessionView?.approvalMode ?? "always-ask"}
              onApprovalModeChange={(mode) => setApprovalMode(selectedSessionId, mode)}
            />
          ) : null}
        </View>
      </KeyboardStickyView>
    </>
  );
}
