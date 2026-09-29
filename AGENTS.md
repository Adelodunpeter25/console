# AGENTS.md — Working Rules

## Commits
- Commit after every task with a **single-line commit message**.
- Stage only the files the task touched; never commit unrelated changes.
- Use `git add <files>` then `git commit -m "<subject>"`.

## Tests & Verification
- Run only the specific test file relevant to the task, e.g.:
  - `cd apps/server-go && go test ./tests/<area>/ -run <TestName> -v`
  - `cd apps/desktop && cargo test -p <crate> <test_name>`
- Never run `run-all-tests.ts` or the full suite unless explicitly asked.
- If a test fails, fix the cause and re-run the same specific test until it passes before committing.
- Never write inline `#[cfg(test)]` modules at the bottom of Rust source files; always place tests in dedicated `tests/` files. Same rule for Kotlin: no test functions inside `app/src/main` sources.
- **The Android app is native Kotlin/Compose at `apps/android`** (not Expo). Verify a change with:
  `cd apps/android && ./gradlew :app:assembleDebug`
  Run its unit tests (if any) with `cd apps/android && ./gradlew :app:testDebugUnitTest`.
  `minifyReleaseWithR8` needs `key.properties`, so only run it when explicitly asked.
- There is currently **no Kotlin test suite** (`apps/android/tests` does not exist) and CI only
  builds Android on manual dispatch. If you add tests, they won't be enforced automatically.

## Scope
- Don't over-engineer. Make the minimal change that satisfies the task.
- Follow existing code patterns and conventions.

## Working Tree & User Changes
- **Never discard, checkout, or reset uncommitted user changes** (`git checkout <file>`, `git restore`, `git reset`, etc.).
- As long as a modified file was not touched by your current task, leave it completely alone.
- Even if user changes cause compilation errors, do NOT revert or fix them without explicit permission; report the compilation error to the user instead.

## Communication & Summaries
- After finishing each task, provide a **short, concise summary in plain English** (2-4 sentences max, no code or technical jargon) describing what broke, how it was fixed, and what to expect.
