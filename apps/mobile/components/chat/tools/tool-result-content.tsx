import React, { useMemo } from "react";
import { View, Text, ScrollView } from "react-native";
import {
  FolderTree,
  Search,
  Globe,
  FilePlus,
  SquarePen,
  HelpCircle,
  CheckCircle2,
  XCircle,
} from "lucide-react-native";
import type { ToolResult } from "@console/types";
import { MarkdownRenderer } from "@/components/common/markdown-renderer";
import { langFromPath, resultText } from "@/utils";

/* ------------------------------------------------------------------ */
/* Status badge — reusable one-liner for write/edit/ask results        */
/* ------------------------------------------------------------------ */

function StatusLine({
  icon: Icon,
  text,
  isError,
}: {
  icon: React.ElementType;
  text: string;
  isError?: boolean;
}) {
  return (
    <View className="flex-row items-center gap-2 py-1">
      <Icon size={13} color={isError ? "#f87171" : "#34d399"} />
      <Text
        className={`text-xs font-mono flex-1 ${
          isError ? "text-red-400" : "text-foreground-secondary"
        }`}
        numberOfLines={2}
        selectable
      >
        {text}
      </Text>
    </View>
  );
}

/* ------------------------------------------------------------------ */
/* Raw fallback                                                        */
/* ------------------------------------------------------------------ */

function RawResult({ text, isError }: { text: string; isError?: boolean }) {
  return (
    <View className="max-h-48 rounded bg-black/40 p-2 overflow-hidden">
      <ScrollView nestedScrollEnabled showsVerticalScrollIndicator={false}>
        <Text
          className={`text-xs font-mono ${
            isError ? "text-red-400" : "text-foreground-secondary"
          }`}
          selectable
        >
          {text}
        </Text>
      </ScrollView>
    </View>
  );
}

/* ------------------------------------------------------------------ */
/* readFile                                                            */
/* ------------------------------------------------------------------ */

function ReadFileResult({ text, filePath }: { text: string; filePath?: string }) {
  const parsed = useMemo(() => {
    const lines = text.split("\n");
    const headerEnd = lines.findIndex((l, i) => i > 0 && l === "");
    const hasHeader = headerEnd > 0 && headerEnd <= 6 && lines[0]?.startsWith("File:");
    const headerLines = hasHeader ? lines.slice(0, headerEnd) : [];
    const codeLines = hasHeader ? lines.slice(headerEnd + 1) : lines;

    const numberedLines = codeLines.filter((line) => /^\s*\d+:\s?/.test(line)).length;
    const normalizedCodeLines =
      numberedLines > codeLines.length / 2
        ? codeLines.map((line) => line.replace(/^\s*\d+:\s?/, ""))
        : codeLines;

    const fileMatch = headerLines.find((l) => l.startsWith("File:"));
    const path = fileMatch?.replace(/^File:\s*/, "") ?? filePath ?? "";
    const lang = langFromPath(path);

    return { lang, normalizedCodeLines, raw: text };
  }, [text, filePath]);

  if (!parsed.lang) return <RawResult text={parsed.raw} />;

  const fence = "```";
  const markdown = `${fence}${parsed.lang}\n${parsed.normalizedCodeLines.join("\n")}\n${fence}`;

  return (
    <View className="max-h-56 overflow-hidden rounded bg-black/30 p-1.5">
      <ScrollView nestedScrollEnabled showsVerticalScrollIndicator={false}>
        <MarkdownRenderer content={markdown} />
      </ScrollView>
    </View>
  );
}

/* ------------------------------------------------------------------ */
/* bash                                                                */
/* ------------------------------------------------------------------ */

function BashResult({ text, isError }: { text: string; isError?: boolean }) {
  const parsed = useMemo(() => {
    const exitMatch = text.match(/^Exit code:\s*(\d+)/);
    const cwdMatch = text.match(/^Working directory:\s*(.+)/m);
    const exitCode = exitMatch ? parseInt(exitMatch[1]!, 10) : undefined;
    const cwd = cwdMatch?.[1];

    const stdoutIdx = text.indexOf("stdout:");
    const stderrIdx = text.indexOf("stderr:");

    if (stdoutIdx === -1) {
      return { hasStdout: false } as const;
    }

    const stdoutStart = stdoutIdx + "stdout:".length;
    const stdoutEnd = stderrIdx > stdoutStart ? stderrIdx : text.length;
    const stdoutText = text.slice(stdoutStart, stdoutEnd).trim();
    const stderrText = stderrIdx > -1 ? text.slice(stderrIdx + "stderr:".length).trim() : "";

    return { hasStdout: true, exitCode, cwd, stdoutText, stderrText } as const;
  }, [text]);

  if (!parsed.hasStdout) {
    return <RawResult text={text} isError={isError} />;
  }

  const { exitCode, cwd, stdoutText, stderrText } = parsed;

  return (
    <View className="gap-1.5">
      <View className="flex-row flex-wrap items-center gap-1.5">
        {exitCode !== undefined && (
          <View
            className={`flex-row items-center gap-1 px-1.5 py-0.5 rounded ${
              exitCode === 0 ? "bg-emerald-500/15" : "bg-red-500/15"
            }`}
          >
            {exitCode === 0 ? (
              <CheckCircle2 size={10} color="#34d399" />
            ) : (
              <XCircle size={10} color="#f87171" />
            )}
            <Text
              className={`text-[10px] font-mono font-bold ${
                exitCode === 0 ? "text-emerald-400" : "text-red-400"
              }`}
            >
              Exit {exitCode}
            </Text>
          </View>
        )}
        {cwd && (
          <Text className="text-[10px] font-mono text-foreground-secondary flex-1" numberOfLines={1}>
            {cwd}
          </Text>
        )}
      </View>
      {stdoutText ? (
        <View>
          <Text className="text-[10px] uppercase tracking-wide text-foreground-secondary/70 mb-0.5 font-medium">
            stdout
          </Text>
          <View className="max-h-40 rounded bg-black/40 p-2 overflow-hidden">
            <ScrollView nestedScrollEnabled showsVerticalScrollIndicator={false}>
              <Text className="text-xs font-mono text-foreground-secondary" selectable>
                {stdoutText}
              </Text>
            </ScrollView>
          </View>
        </View>
      ) : null}
      {stderrText ? (
        <View>
          <Text className="text-[10px] uppercase tracking-wide text-foreground-secondary/70 mb-0.5 font-medium">
            stderr
          </Text>
          <View className="max-h-40 rounded bg-red-500/5 p-2 overflow-hidden">
            <ScrollView nestedScrollEnabled showsVerticalScrollIndicator={false}>
              <Text
                className={`text-xs font-mono ${isError ? "text-red-400" : "text-foreground-secondary"}`}
                selectable
              >
                {stderrText}
              </Text>
            </ScrollView>
          </View>
        </View>
      ) : null}
    </View>
  );
}

/* ------------------------------------------------------------------ */
/* listDir                                                             */
/* ------------------------------------------------------------------ */

function ListDirResult({ text }: { text: string }) {
  const parsed = useMemo(() => {
    const lines = text.split("\n");
    const dirMatch = lines[0]?.match(/^Directory:\s*(.+)/);
    const dirPath = dirMatch?.[1] ?? "";
    const treeLines = lines.slice(1);
    return { dirPath, treeText: treeLines.join("\n") };
  }, [text]);

  return (
    <View className="gap-1.5">
      {parsed.dirPath ? (
        <View className="flex-row items-center gap-1.5">
          <FolderTree size={11} color={theme.colors.text.muted} />
          <Text className="text-[10px] font-mono text-foreground-secondary flex-1" numberOfLines={1}>
            {parsed.dirPath}
          </Text>
        </View>
      ) : null}
      <View className="max-h-48 rounded bg-black/40 p-2 overflow-hidden">
        <ScrollView nestedScrollEnabled showsVerticalScrollIndicator={false}>
          <Text className="text-xs font-mono text-foreground-secondary" selectable>
            {parsed.treeText}
          </Text>
        </ScrollView>
      </View>
    </View>
  );
}

/* ------------------------------------------------------------------ */
/* grep                                                                */
/* ------------------------------------------------------------------ */

function GrepResult({ text }: { text: string }) {
  const parsed = useMemo(() => {
    const lines = text.split("\n");
    const headerMatch = lines[0]?.match(/^Found\s+(\d+)\s+match/);
    if (!headerMatch) return null;
    const header = lines[0];
    const bodyLines = lines.slice(1).filter((l) => l !== "");
    return { header, bodyText: bodyLines.join("\n") };
  }, [text]);

  if (!parsed) return <RawResult text={text} />;

  return (
    <View className="gap-1.5">
      <View className="flex-row items-center gap-1.5">
        <Search size={11} color={theme.colors.text.muted} />
        <Text className="text-[10px] font-mono text-foreground-secondary">{parsed.header}</Text>
      </View>
      <View className="max-h-48 rounded bg-black/40 p-2 overflow-hidden">
        <ScrollView nestedScrollEnabled showsVerticalScrollIndicator={false}>
          <Text className="text-xs font-mono text-foreground-secondary" selectable>
            {parsed.bodyText}
          </Text>
        </ScrollView>
      </View>
    </View>
  );
}

/* ------------------------------------------------------------------ */
/* glob                                                                */
/* ------------------------------------------------------------------ */

function GlobResult({ text }: { text: string }) {
  const parsed = useMemo(() => {
    const lines = text.split("\n");
    const headerMatch = lines[0]?.match(/^Found\s+(\d+)\s+file/);
    if (!headerMatch) return null;
    const header = lines[0];
    const files = lines.slice(1).filter((l) => l.trim());
    return { header, filesText: files.join("\n") };
  }, [text]);

  if (!parsed) return <RawResult text={text} />;

  return (
    <View className="gap-1.5">
      <View className="flex-row items-center gap-1.5">
        <FolderTree size={11} color={theme.colors.text.muted} />
        <Text className="text-[10px] font-mono text-foreground-secondary">{parsed.header}</Text>
      </View>
      <View className="max-h-40 rounded bg-black/40 p-2 overflow-hidden">
        <ScrollView nestedScrollEnabled showsVerticalScrollIndicator={false}>
          <Text className="text-xs font-mono text-foreground-secondary" selectable>
            {parsed.filesText}
          </Text>
        </ScrollView>
      </View>
    </View>
  );
}

/* ------------------------------------------------------------------ */
/* fetch / webSearch                                                   */
/* ------------------------------------------------------------------ */

function WebSearchResult({ text }: { text: string }) {
  return (
    <View className="max-h-56 rounded bg-black/30 p-2 overflow-hidden">
      <ScrollView nestedScrollEnabled showsVerticalScrollIndicator={false}>
        <MarkdownRenderer content={text} />
      </ScrollView>
    </View>
  );
}

function FetchResult({ text }: { text: string }) {
  const parsed = useMemo(() => {
    const bodyIdx = text.indexOf("Body:\n");
    const headerText = bodyIdx > -1 ? text.slice(0, bodyIdx) : "";
    const bodyText = bodyIdx > -1 ? text.slice(bodyIdx + "Body:\n".length) : text;

    const urlMatch = headerText.match(/^URL:\s*(.+)/m);
    const statusMatch = headerText.match(/^Status:\s*(\d+)/m);

    const bodyIsJson = bodyText.trim().startsWith("{") || bodyText.trim().startsWith("[");
    const renderedBody = bodyIsJson ? "```json\n" + bodyText + "\n```" : bodyText;

    return { url: urlMatch?.[1], status: statusMatch?.[1], renderedBody };
  }, [text]);

  return (
    <View className="gap-1.5">
      <View className="flex-row flex-wrap items-center gap-1.5">
        <Globe size={11} color={theme.colors.text.muted} />
        {parsed.url ? (
          <Text className="text-[10px] font-mono text-foreground-secondary flex-1" numberOfLines={1}>
            {parsed.url}
          </Text>
        ) : null}
        {parsed.status ? (
          <View
            className={`px-1 rounded ${
              parseInt(parsed.status, 10) < 400 ? "bg-emerald-500/10" : "bg-red-500/10"
            }`}
          >
            <Text
              className={`text-[10px] font-mono ${
                parseInt(parsed.status, 10) < 400 ? "text-emerald-400" : "text-red-400"
              }`}
            >
              {parsed.status}
            </Text>
          </View>
        ) : null}
      </View>
      <View className="max-h-56 rounded bg-black/30 p-2 overflow-hidden">
        <ScrollView nestedScrollEnabled showsVerticalScrollIndicator={false}>
          <MarkdownRenderer content={parsed.renderedBody} />
        </ScrollView>
      </View>
    </View>
  );
}

import { DiffView } from "./diff-view";
import { computeLineDiff, computeNewFileDiff } from "@/utils/diff";
import { theme } from "@/styles/theme";

/* ------------------------------------------------------------------ */
/* writeFile / editFile / ask                                          */
/* ------------------------------------------------------------------ */

function WriteFileResult({ text, isError }: { text: string; isError?: boolean }) {
  const firstLine = useMemo(() => text.split("\n")[0] ?? text, [text]);
  return <StatusLine icon={isError ? XCircle : FilePlus} text={firstLine} isError={isError} />;
}

function EditFileResult({ text, isError }: { text: string; isError?: boolean }) {
  const fullText = useMemo(() => {
    const firstLine = text.split("\n")[0] ?? text;
    const summary = text.split("\n")[1]?.trim();
    return summary ? `${firstLine} — ${summary}` : firstLine;
  }, [text]);
  return <StatusLine icon={isError ? XCircle : SquarePen} text={fullText} isError={isError} />;
}

function AskResult({ text }: { text: string }) {
  return <StatusLine icon={HelpCircle} text={text} />;
}

/* ------------------------------------------------------------------ */
/* Main ToolResultContent Component                                    */
/* ------------------------------------------------------------------ */

interface ToolResultContentProps {
  toolName?: string;
  result: ToolResult;
  callFilePath?: string;
  callArgs?: unknown;
}

export function ToolResultContent({ toolName, result, callFilePath, callArgs }: ToolResultContentProps) {
  const text = resultText(result);
  const isError = result.isError;

  if (isError) {
    switch (toolName) {
      case "writeFile":
        return <WriteFileResult text={text} isError />;
      case "editFile":
        return <EditFileResult text={text} isError />;
      default:
        return <RawResult text={text} isError />;
    }
  }

  // Check if we have file-diff arguments available for editFile
  if (toolName === "editFile" && callArgs && typeof callArgs === "object") {
    const args = callArgs as Record<string, unknown>;
    if (typeof args.oldContent === "string" && typeof args.newContent === "string") {
      const diff = computeLineDiff(args.oldContent, args.newContent);
      const filePath = typeof args.path === "string" ? args.path : callFilePath;
      return <DiffView diff={diff} filePath={filePath} />;
    }
  }

  // Check if we have file content available for writeFile
  if (toolName === "writeFile" && callArgs && typeof callArgs === "object") {
    const args = callArgs as Record<string, unknown>;
    if (typeof args.content === "string") {
      const diff = computeNewFileDiff(args.content);
      const filePath = typeof args.path === "string" ? args.path : callFilePath;
      return <DiffView diff={diff} filePath={filePath} />;
    }
  }

  // Check if we have batch files available for batchWrite
  if (toolName === "batchWrite" && callArgs && typeof callArgs === "object") {
    const args = callArgs as Record<string, unknown>;
    if (Array.isArray(args.files) && args.files.length > 0) {
      return (
        <View className="gap-2">
          {args.files.map((file: unknown, index: number) => {
            if (!file || typeof file !== "object") return null;
            const f = file as Record<string, unknown>;
            const filePath = typeof f.path === "string" ? f.path : `file-${index + 1}`;
            const content = typeof f.content === "string" ? f.content : "";
            const diff = computeNewFileDiff(content);
            return <DiffView key={filePath} diff={diff} filePath={filePath} />;
          })}
        </View>
      );
    }
  }

  switch (toolName) {
    case "readFile":
      return <ReadFileResult text={text} filePath={callFilePath} />;
    case "bash":
      return <BashResult text={text} />;
    case "listDir":
      return <ListDirResult text={text} />;
    case "grep":
      return <GrepResult text={text} />;
    case "glob":
      return <GlobResult text={text} />;
    case "webSearch":
      return <WebSearchResult text={text} />;
    case "fetch":
      return <FetchResult text={text} />;
    case "writeFile":
    case "batchWrite":
      return <WriteFileResult text={text} />;
    case "editFile":
      return <EditFileResult text={text} />;
    case "ask":
      return <AskResult text={text} />;
    default:
      return <RawResult text={text} />;
  }
}
