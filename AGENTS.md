# AGENTS.md — Working Rules

## Commits
- Commit after every task with a **single-line commit message**.
- Stage only the files the task touched; never commit unrelated changes.
- Use `git add <files>` then `git commit -m "<subject>"`.

## Tests & Verification
- Run only the specific test file relevant to the task, e.g.:
  `cd apps/server && bun tests/<name>.test.ts`
- Never run `run-all-tests.ts` or the full suite unless explicitly asked.
- If a test fails, fix the cause and re-run the same specific test until it passes before committing.
- Never write inline `#[cfg(test)]` modules at the bottom of Rust source files; always place tests in dedicated `tests/` files.
- For mobile bundling verification:
  `cd apps/mobile && bunx expo export --platform android`
- For mobile icon generation:
  `bash apps/mobile/scripts/generate-icons.sh [path/to/icon.png]`

## Scope
- Don't over-engineer. Make the minimal change that satisfies the task.
- Follow existing code patterns and conventions.

## Working Tree & User Changes
- **Never discard, checkout, or reset uncommitted user changes** (`git checkout <file>`, `git restore`, `git reset`, etc.).
- As long as a modified file was not touched by your current task, leave it completely alone.
- Even if user changes cause compilation errors, do NOT revert or fix them without explicit permission; report the compilation error to the user instead.

## Communication & Summaries
- After finishing each task, always provide a clear explanation in **plain English** (not code or technical jargon) describing what the problem was, how it was solved, and what the user should expect.
