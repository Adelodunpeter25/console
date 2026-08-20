# AGENTS.md — Working Rules

## Commits
- Commit after every task with a **single-line commit message**.
- Stage only the files the task touched; never commit unrelated changes.
- Use `git add <files>` then `git commit -m "<subject>"`.

## Tests & Verification
- Run only the specific test file relevant to the task, e.g.:
  `cd apps/server && npx tsx tests/<name>.test.ts`
- Never run `run-all-tests.ts` or the full suite unless explicitly asked.
- If a test fails, fix the cause and re-run the same specific test until it passes before committing.
- For mobile bundling verification:
  `cd apps/mobile && npx expo export --platform android`
- For mobile icon generation:
  `bash apps/mobile/scripts/generate-icons.sh [path/to/icon.png]`

## Scope
- Don't over-engineer. Make the minimal change that satisfies the task.
- Follow existing code patterns and conventions.
