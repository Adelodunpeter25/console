# Cross-platform command sandbox

Build a project-owned sandbox layer so commands can be restricted consistently on
macOS, Windows, and Linux. The application should ask for a policy such as
`read-only` or `workspace`, while each operating system uses its own available
process and filesystem restrictions underneath.

The goal is not to invent a new operating system security mechanism. The goal is
to give the rest of the application one small interface and to keep the
platform-specific work in separate backends.

## 1. What a sandbox means here

A sandbox starts a command with limits. For example:

- A **read-only** command can inspect files but cannot change the project.
- A **workspace** command can change the selected project but cannot change
  unrelated user files.
- A **full-access** command can use the user's normal permissions, while still
  being managed and tracked by the application.

The sandbox must not silently claim to be protecting a command when the current
machine cannot enforce the requested policy.

## 2. Non-goals for the first version

- Do not build a virtual machine or a complete operating system container.
- Do not make every platform provide identical security guarantees.
- Do not require users to manually install one particular tool as the only way
  to use the application.
- Do not silently fall back from a required sandbox to an unrestricted process.
- Do not start by implementing every possible restriction, such as perfect
  network filtering or every kernel security feature.

## 3. Common policy and runner interface

Add a small internal abstraction between command tools and process creation. The
command tools should request a policy rather than constructing `bwrap`,
`sandbox-exec`, or Windows-specific process settings themselves.

A policy should eventually describe:

- mode: `read-only`, `workspace`, or `full-access`;
- working directory;
- paths that may be read;
- paths that may be written;
- whether network access is allowed;
- whether child processes are allowed;
- environment variables to pass through;
- timeout, memory, CPU, and process-count limits.

The first implementation can support only the fields that each backend can
actually enforce. Unsupported fields must be reported as capabilities or
limitations rather than being ignored silently.

The runner should provide operations equivalent to:

- run a command and capture its output;
- start a managed background command;
- stop a command and its children;
- report whether a requested policy is available on this machine.

## 4. Platform backends

### Linux

Start with the existing bubblewrap backend, but move it behind the common
runner interface. It should be one Linux implementation, not the public API of
the application.

Later, consider shipping a small Linux helper that uses kernel features directly
when that is worthwhile. This still depends on the VM allowing those features.
If the kernel does not permit the requested isolation, the backend must return a
clear unavailable result.

### macOS

Keep the existing read-only implementation as the first backend. It starts the
command with macOS sandbox rules that deny writes except in temporary folders.

The platform backend should hide the command-line details from the rest of the
server. A future native helper can replace the current launcher if the existing
macOS mechanism becomes unreliable or insufficient.

### Windows

Begin with process management and conservative restrictions:

- use a Job Object so the command and its child processes can be stopped as one
  group;
- enforce time and resource limits where practical;
- start the command with a restricted process identity when it is reliable;
- grant access to the selected workspace explicitly where the chosen Windows
  security mechanism supports it.

Treat Windows workspace-only filesystem protection as a capability that may be
partial in the first version. Do not label it fully isolated until it has tests
proving that unrelated files cannot be changed.

## 5. Failure and fallback rules

There are two different situations:

1. **The user requested isolation.** If the backend cannot enforce it, refuse to
   start the command and explain why.
2. **The user explicitly requested full access.** Start the command normally,
   but continue to apply process tracking, timeout handling, output limits, and
   cleanup.

This distinction prevents a missing dependency or disabled VM feature from
turning a supposedly safe plan-mode command into an unrestricted command.

## 6. Integration with the existing server

Replace direct use of `sandboxArgv` in the bash tool with the common runner.
Keep the existing command behavior—working directory, environment handling,
timeouts, output truncation, aborts, and background jobs—but make the runner
responsible for platform-specific process setup.

Plan mode should request `read-only`.

Full Access mode should request `workspace` initially, not unrestricted host
access, when the user has selected a project. A separate explicit option can
request `full-access` if that is the intended behavior.

The runner should return enough information for tool output to say:

- which policy was applied;
- whether any requested capability was unavailable;
- why a command was refused;
- how the command was stopped or timed out.

## 7. Testing strategy

Tests should verify behavior, not just whether a platform executable exists.

Shared tests should cover:

- a command can read an allowed file;
- a read-only command cannot modify the project;
- a workspace command can modify the selected project;
- an unavailable required policy fails closed;
- full-access mode still has timeout and process cleanup;
- child processes are stopped with their parent;
- environment and output limits continue to work.

Platform-specific tests should be skipped only when the platform capability is
not available, and should clearly report that reason. A test must not pass by
silently running the command without a sandbox.

## 8. Suggested implementation order

1. Define the policy, capability, result, and runner types.
2. Move the current Linux and macOS launchers behind platform backends without
   changing user-visible behavior.
3. Change sandbox availability checks to return a reason and capabilities.
4. Add fail-closed behavior for required read-only and workspace policies.
5. Add managed process cleanup shared by foreground and background commands.
6. Implement the first Windows backend with Job Objects and conservative process
   restrictions.
7. Add shared and platform-specific behavior tests.
8. Add workspace/full-access policy support after the read-only path is reliable.

## 9. Gaps to resolve before implementation

The first draft describes the direction, but these decisions must be explicit
before code is written.

### 9.1 Define the security promise

“Read-only” and “workspace” are security promises, not just labels. Document the
threat model first:

- the command may be malicious or may contain model-generated mistakes;
- the command may start child processes, follow symlinks, inspect environment
  variables, and use interpreters already installed on the machine;
- the command must not escape the approved paths through `..`, symlinks,
  bind mounts, junctions, or alternate path spellings;
- credentials, SSH agents, desktop sockets, and other sensitive handles are not
  automatically available merely because the command runs as the user.

For each policy, state what is protected and what is deliberately not protected.
The implementation and tests should use that statement as the contract.

### 9.2 Separate policy from capability

A requested policy is not the same thing as what a platform can enforce. Define
stable result types for:

- requested policy;
- effective policy;
- supported capabilities;
- degraded or unavailable capabilities;
- refusal reason.

A backend must either enforce the required fields or refuse the request. It must
never quietly downgrade `workspace` to `full-access`. If a caller explicitly
allows a weaker policy, that downgrade should be visible in the result and in
logs.

### 9.3 Clarify “full access”

Full access should not mean that every command is trusted. It means the command
may use the user's normal filesystem permissions. The runner should still apply
lifetime, output, process-tree cleanup, and resource limits.

Also decide where the user makes this choice, how it is displayed, and whether a
session can change policy while a command is running. A policy must be fixed for
the lifetime of a process; changing it requires starting a new process.

### 9.4 Make path rules precise

Define whether the approved workspace includes the workspace itself, hidden
files, generated files, mounted repositories, and temporary directories. Resolve
and validate paths before launch, but also enforce them inside the OS backend so
validation alone is not vulnerable to a race.

Tests must cover symlinks, renamed directories, junctions on Windows, hard links
where relevant, relative paths, and commands launched from outside the workspace.
Do not expose host-wide read access by default just because the command needs
system libraries to start.

### 9.5 Treat network and secrets as separate policies

“Network allowed” is too broad for a first API. Decide whether it means all
network access, no network access, or a future allowlist. Make credentials a
separate policy: inherited environment variables, SSH agent forwarding, cloud
credentials, and browser/session sockets should be opt-in and clearly shown to
the user.

### 9.6 Specify the Windows starting point honestly

A Job Object controls process lifetime and resources; by itself it does not
sandbox filesystem access. A restricted token also does not automatically make a
workspace-only filesystem boundary. Choose and document the first Windows
filesystem mechanism, or explicitly mark the Windows backend as process-managed
rather than filesystem-isolated.

The product must not call that backend “workspace sandboxing” until tests prove
that it prevents writes outside the workspace. If strong filesystem isolation is
not available, fail closed for policies that require it.

### 9.7 Define process and shell behavior

Specify whether the runner launches `/bin/sh`, `bash`, PowerShell, or `cmd.exe`,
and how arguments and environment values are passed. Keep the command and policy
separate; never build a shell command by concatenating untrusted policy values.

Define behavior for descendants, daemons, detached processes, process groups,
stdin, terminal allocation, signals, and background jobs. A timeout is incomplete
if a child process survives it or retains a writable handle.

### 9.8 Plan helper distribution and trust

If a native helper is needed, decide how it is built, signed, installed, updated,
and versioned on each platform. The server should verify that the helper speaks a
compatible protocol and should fail safely when it is missing or altered. Avoid
requiring a compiler or administrator access at runtime.

### 9.9 Add operational and compatibility requirements

Record the effective policy and backend in structured logs without recording
secrets. Return a stable machine-readable error so the desktop can explain a
refusal and offer the user an intentional alternative. Keep older clients
compatible while adding policy fields, and define what happens after a server
restart to running jobs.

### 9.10 Use security review gates

Before enabling a backend by default, review its launch arguments, environment,
file descriptor inheritance, path handling, and child-process cleanup. Add a
platform-specific adversarial test suite and document known limitations. A
backend should be promoted from experimental only after its required policy is
proven on supported OS versions, not merely when its launcher executable exists.

## 10. Suggested implementation order

1. Write the threat model and policy contract, including path, network, secret,
   and process semantics.
2. Define policy, capability, result, error, and runner types.
3. Move the current Linux and macOS launchers behind platform backends without
   changing user-visible behavior.
4. Change availability checks to return a reason and capabilities.
5. Add fail-closed behavior for required read-only and workspace policies.
6. Add shared process-tree cleanup, resource limits, and structured results.
7. Decide and prototype the Windows filesystem backend; do not promise workspace
   isolation until its behavior tests pass.
8. Add shared and platform-specific adversarial tests.
9. Add workspace/full-access UI and policy selection after the read-only path is
   reliable.
10. Document helper packaging, upgrades, support boundaries, and recovery of
    running jobs.

## 11. Definition of success

The server has one platform-independent way to request a command policy. Linux,
macOS, and Windows each have a backend that reports what it can enforce. Plan
mode never silently becomes unrestricted because a dependency is missing, and
full-access commands remain bounded, tracked, and cleanly stoppable.

The product documentation also states the exact security promise for every
policy, exposes effective capabilities to the user, handles paths and secrets
explicitly, and refuses policies that a platform cannot actually enforce.
