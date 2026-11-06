// One-off generator: run `node scripts/generate-icons.mjs` from the repo root
// after copying new SVGs into apps/console-nsh/assets/icons. Writes
// apps/console-nsh/src/common/icons.generated.ts.
import { execSync } from "node:child_process";
import fs from "node:fs";

const root = "apps/console-nsh/assets/icons";
const files = execSync(`find ${root} -name "*.svg" | sort`)
  .toString()
  .trim()
  .split("\n");
const pascal = (s) =>
  s.replace(/(^|[-_])([a-z0-9])/g, (_, __, c) => c.toUpperCase());

const imports = [];
const entries = [];
for (const f of files) {
  const sub = f.startsWith(root + "/") ? f.slice(root.length + 1) : f;
  const base = sub.split("/").pop().replace(/\.svg$/, "");
  const varName = "asset_" + sub.replace(/[/.-]/g, "_").replace(/_svg$/, "");
  const key = sub.startsWith("providers/")
    ? "provider/" + pascal(base)
    : sub.startsWith("file-types/")
      ? "fileType/" + pascal(base)
      : pascal(base);
  imports.push(`import ${varName} from "../../assets/icons/${sub}";`);
  entries.push(`  "${key}": ${varName},`);
}

const header = [
  "// Generated from apps/desktop/assets/icons -- do not edit by hand.",
  "// Every icon of the GPUI app, keyed the way console-ui's icons.rs maps",
  "// them: the PascalCase name (ChatRoundLine), provider logos under",
  "// provider/Name, file-type logos under fileType/Name. Each value is the",
  "// imported ImageAsset the bundler ships with the app.",
  "",
];
const out =
  header.join("\n") +
  imports.join("\n") +
  "\n\nexport const ICON_ASSETS = {\n" +
  entries.join("\n") +
  "\n} as const;\n\nexport type IconName = keyof typeof ICON_ASSETS;\n";
fs.writeFileSync("apps/console-nsh/src/common/icons.generated.ts", out);
console.log("wrote", files.length, "icons");
