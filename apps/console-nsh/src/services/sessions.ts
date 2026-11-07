// Centralized session management service: lifecycle, settle/unsettle, delete, restore, and fork.

import { Console, type SessionSummary } from "console-nsh/native";

async function toggleSettle(
  baseUrl: string,
  session: SessionSummary,
): Promise<void> {
  if (session.status === "settled") {
    await Console.unsettleSession(baseUrl, session.id);
  } else {
    await Console.settleSession(baseUrl, session.id);
  }
}

async function deleteSession(baseUrl: string, sessionId: string): Promise<void> {
  await Console.deleteSession(baseUrl, sessionId);
}

async function restoreSession(baseUrl: string, sessionId: string): Promise<void> {
  await Console.restoreSession(baseUrl, sessionId);
}

async function forkSession(
  baseUrl: string,
  sessionId: string,
  messageId: string,
): Promise<string> {
  const raw = await Console.forkSession(baseUrl, sessionId, messageId);
  const header = JSON.parse(raw) as { id: string };
  return header.id;
}

async function createSession(
  baseUrl: string,
  projectId: string,
  title?: string | null,
): Promise<{ id: string }> {
  const raw = await Console.createSession(baseUrl, projectId, title);
  return JSON.parse(raw) as { id: string };
}

export const SessionService = {
  toggleSettle,
  delete: deleteSession,
  restore: restoreSession,
  fork: forkSession,
  create: createSession,
};
