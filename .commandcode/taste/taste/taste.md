# Taste
- Prefers to commit the current working changes before diagnosing or fixing unrelated issues (e.g., asked to "commit first" before discussing what's wrong). Confidence: 0.9
- Prefers to use bun for running scripts such as typecheck and build. Confidence: 0.9
- Prefers to verify typecheck/build passes before pushing. Confidence: 0.7
- When a lockfile is out of sync, prefers to regenerate it with the package manager (e.g., `bun install`) rather than skip or manually inspect. Confidence: 0.8
- Prefers a direct, action-first approach over extended diagnostic explanations; expects requested actions (e.g., `git push`) to be executed without repeated confirmation. Confidence: 0.8
- Communicates in terse, typo-heavy shorthand; expects the assistant to infer intent and act without asking for clarification. Confidence: 0.7
- When adopting a new library/tool, prefers a full uniform replacement over a scoped or hybrid adoption (e.g., chose to replace react-markdown everywhere rather than only in the streaming bubble). Confidence: 0.5
- When asked to "check out" or investigate an issue, wants a diagnosis first; implement fixes only after approval. Confidence: 0.7
- Commits completed fixes/changes by default, but pushes only when explicitly told to — an explicit "push only when I tell you" overrides any earlier "commit and push when done" instruction. Confidence: 0.8
 task is done (e.g., "commit and push when you are done") — committing/pushing is part of finishing the work, not a separate request. Confidence: 0.8
- Opposed to language/framework rewrites or adoptions driven by trend; wants technical decisions justified by genuine need and measured impact (explicitly framed Rust rewrite as "not because of trend or because I actually need it"). Confidence: 0.8
- Treats low runtime memory/resource footprint as a real project constraint for the server (designed as a remote server where "every megabyte counts"); expects architecture discussions to weigh that constraint against actual costs. Confidence: 0.7
