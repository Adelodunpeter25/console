/**
 * Env command - Manage provider API keys for the daemon.
 *
 * Walks the known service keys one prompt per line; pasted input stays
 * hidden. Empty input keeps the existing value. Keys land in ~/.console/env
 * (mode 0600), which `console start` merges under explicit process env —
 * so every current and future daemon picks them up with no shell exports.
 * Piped stdin works too: `echo "$KEY" | console env`.
 */
import * as readline from "node:readline";
import { loadEnvFile, upsertEnvValues, getEnvFilePath } from "../daemon-manager.js";

interface ServiceKey {
  /** Short name shown in prompts. */
  service: string;
  /** Canonical env var this service reads. */
  envVar: string;
}

const SERVICE_KEYS: ServiceKey[] = [{ service: "firecrawl", envVar: "FIRECRAWL_API_KEY" }];

function isValidValue(value: string): boolean {
  return value.length > 0 && !/[\r\n"']/.test(value);
}

async function readHiddenLine(): Promise<string> {
  // Piped input (fleet runs): first line, no masking needed.
  if (!process.stdin.isTTY) {
    const rl = readline.createInterface({ input: process.stdin });
    let first = "";
    for await (const line of rl) {
      first = line;
      break;
    }
    rl.close();
    return first.trim();
  }
  // TTY: raw mode with * masking so a pasted secret never shows.
  return new Promise((resolve) => {
    const stdin = process.stdin;
    stdin.setRawMode(true);
    stdin.resume();
    let out = "";
    const cleanup = () => {
      stdin.off("data", onData);
      stdin.setRawMode(false);
      stdin.pause();
      process.stdout.write("\n");
    };
    const onData = (buf: Buffer) => {
      for (const ch of buf.toString("utf8")) {
        if (ch === "\n" || ch === "\r" || ch === "\u0004") {
          cleanup();
          resolve(out.trim());
          return;
        }
        if (ch === "\u0003") {
          cleanup();
          process.exit(130);
        }
        if (ch === "\u007f" || ch === "\b") {
          if (out.length > 0) {
            out = out.slice(0, -1);
            process.stdout.write("\b \b");
          }
          continue;
        }
        out += ch;
        process.stdout.write("*");
      }
    };
    stdin.on("data", onData);
  });
}

export async function envCommand(): Promise<void> {
  const existing = await loadEnvFile();
  const changed: Record<string, string> = {};

  for (const { service, envVar } of SERVICE_KEYS) {
    const has = existing[envVar] ? " (already set — Enter to keep)" : "";
    process.stdout.write(`${envVar}${has}:\n> `);
    const value = await readHiddenLine();
    if (!value) continue;
    if (!isValidValue(value)) {
      console.error(`Skipping ${service}: value must be one line with no quotes.`);
      continue;
    }
    changed[envVar] = value;
  }

  if (Object.keys(changed).length === 0) {
    console.log("Nothing changed.");
    return;
  }
  await upsertEnvValues(changed);
  console.log(`Saved to ${getEnvFilePath()}. Run 'console restart' to apply.`);
}
