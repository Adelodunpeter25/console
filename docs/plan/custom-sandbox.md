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

## 9. Definition of success

The server has one platform-independent way to request a command policy. Linux,
macOS, and Windows each have a backend that reports what it can enforce. Plan
mode never silently becomes unrestricted because a dependency is missing, and
full-access commands remain bounded, tracked, and cleanly stoppable.
