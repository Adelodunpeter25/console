package codemode

// bootstrapScript initializes the ergonomic JavaScript primitives mirroring
// Claude Code's REPL environment. It connects to the underlying __harness bridge.
const bootstrapScript = `
(function() {
    const bridge = globalThis.__harness;

    // tools.<name>(input) dynamic Proxy
    globalThis.tools = new Proxy({}, {
        get(target, prop) {
            if (typeof prop === "symbol" || prop === "then" || prop === "toJSON" || prop === "constructor") {
                return undefined;
            }
            return async function(input) {
                return await bridge.call({ tool: String(prop), input: input || {} });
            };
        }
    });

    // File primitives
    // file(path, [startLine], [endLine]) or file(path, { startLine, endLine }).
    globalThis.file = async function(path, a, b) {
        const input = { path: String(path) };
        if (a && typeof a === "object") {
            Object.assign(input, a);
        } else {
            if (a) input.startLine = a;
            if (b) input.endLine = b;
        }
        const res = await bridge.call({ tool: "readFile", input: input });
        if (typeof res === "string") return res;
        if (Array.isArray(res) && res.length > 0 && res[0].text !== undefined) {
            return res[0].text;
        }
        if (res && typeof res.text === "string") return res.text;
        return typeof res === "object" ? JSON.stringify(res) : String(res);
    };
    globalThis.readFile = globalThis.file;

    // fileRaw(path): exact file bytes, no header or line numbers. Use this (not
    // file()) for read-modify-write.
    globalThis.fileRaw = async function(path, opts) {
        return await globalThis.file(path, Object.assign({}, opts || {}, { raw: true }));
    };
    globalThis.readRaw = globalThis.fileRaw;

    globalThis.write = async function(path, content) {
        return await bridge.call({ tool: "writeFile", input: { path: String(path), content: String(content) } });
    };
    globalThis.writeFile = globalThis.write;

    globalThis.edit = async function(path, oldContent, newContent) {
        return await bridge.call({
            tool: "editFile",
            input: {
                path: String(path),
                oldContent: String(oldContent),
                newContent: String(newContent)
            }
        });
    };
    globalThis.editFile = globalThis.edit;

    globalThis.glob = async function(pattern, root) {
        const input = { pattern: String(pattern) };
        if (root && typeof root === "object") {
            Object.assign(input, root);
        } else if (root) {
            input.root = String(root);
        }
        const res = await bridge.call({ tool: "glob", input: input });
        let text = "";
        if (typeof res === "string") {
            text = res;
        } else if (Array.isArray(res) && res[0] && res[0].text) {
            text = res[0].text;
        } else {
            text = String(res);
        }
        const lines = text.split("\n").map(l => l.trim()).filter(Boolean);
        if (lines.length > 0 && /^(Matched|Found) \d+ file/.test(lines[0])) {
            return lines.slice(1);
        }
        if (lines.length === 1 && lines[0].startsWith("No files matched")) {
            return [];
        }
        return lines;
    };

    globalThis.grep = async function(pattern, options) {
        const input = Object.assign({ pattern: String(pattern) }, options || {});
        return await bridge.call({ tool: "grep", input: input });
    };

    globalThis.list = async function(path, options) {
        return await bridge.call({ tool: "listDir", input: Object.assign({ path: String(path || ".") }, options || {}) });
    };
    globalThis.listDir = globalThis.list;

    // Web primitives
    globalThis.fetch = async function(url, options) {
        const input = typeof url === "object" ? url : Object.assign({ url: String(url) }, options || {});
        const res = await bridge.call({ tool: "webFetch", input: input });
        if (typeof res === "string") return res;
        if (Array.isArray(res) && res.length > 0 && res[0].text !== undefined) {
            return res[0].text;
        }
        if (res && typeof res.text === "string") return res.text;
        return typeof res === "object" ? JSON.stringify(res) : String(res);
    };
    globalThis.webFetch = globalThis.fetch;

    globalThis.webSearch = async function(query, options) {
        const input = typeof query === "object" ? query : Object.assign({ query: String(query) }, options || {});
        const res = await bridge.call({ tool: "webSearch", input: input });
        if (typeof res === "string") return res;
        if (Array.isArray(res) && res.length > 0 && res[0].text !== undefined) {
            return res[0].text;
        }
        return typeof res === "object" ? JSON.stringify(res) : String(res);
    };

    // Todo list helper
    globalThis.todo = async function(op, arg) {
        const input = { op: String(op) };
        if (typeof arg === "number") {
            input.index = arg;
        } else if (Array.isArray(arg)) {
            input.tasks = arg;
        } else if (typeof arg === "string") {
            input.tasks = [arg];
        } else if (typeof arg === "object" && arg !== null) {
            Object.assign(input, arg);
        }
        return await bridge.call({ tool: "todo", input: input });
    };

    // batchWrite(files, [options]): files is [{path, content}, ...].
    globalThis.batchWrite = async function(files, options) {
        const input = Object.assign({ files: files }, options || {});
        return await bridge.call({ tool: "batchWrite", input: input });
    };

    // memory(op, [options]): op is store|recall|list|edit|delete.
    globalThis.memory = async function(op, options) {
        const input = typeof op === "object" && op !== null ? op : Object.assign({ op: String(op) }, options || {});
        return await bridge.call({ tool: "memory", input: input });
    };

    // Artifacts & chat reference
    globalThis.render = async function(input) {
        return await bridge.call({ tool: "render", input: input });
    };
    globalThis.renderPreview = async function(input) {
        return await bridge.call({ tool: "preview", input: input });
    };

    globalThis.readChat = async function(ref, limit) {
        const input = { ref: String(ref) };
        if (limit && typeof limit === "object") {
            Object.assign(input, limit);
        } else if (limit) {
            input.limit = limit;
        }
        return await bridge.call({ tool: "readChat", input: input });
    };

    globalThis.readSkill = async function(name, options) {
        const input = Object.assign(name ? { name: String(name) } : {}, options || {});
        return await bridge.call({ tool: "readSkill", input: input });
    };

    // Plan mode design plan writer
    globalThis.writePlan = async function(arg1, arg2) {
        let input = {};
        if (typeof arg1 === "object" && arg1 !== null) {
            input = arg1;
        } else {
            input = { name: String(arg1 || ""), content: String(arg2 || "") };
        }
        return await bridge.call({ tool: "writePlan", input: input });
    };

    // Plan mode upgrade to Full Access
    globalThis.requestFullAccess = async function(reason) {
        let input = {};
        if (typeof reason === "object" && reason !== null) {
            input = reason;
        } else {
            input = { reason: String(reason || "") };
        }
        return await bridge.call({ tool: "requestFullAccess", input: input });
    };

    // User interaction: ask clarifying questions
    globalThis.ask = async function(question, options, isMultiSelect) {
        let input = {};
        if (typeof question === "object" && question !== null) {
            input = question;
        } else {
            input = { question: String(question || "") };
            if (Array.isArray(options)) {
                input.options = options.map(String);
            } else if (options && typeof options === "object") {
                Object.assign(input, options);
            }
            if (isMultiSelect) {
                input.isMultiSelect = true;
            }
        }
        return await bridge.call({ tool: "ask", input: input });
    };

    globalThis.askMany = async function(questions) {
        const input = Array.isArray(questions) ? { questions: questions } : (questions || {});
        return await bridge.call({ tool: "askMany", input: input });
    };

    // Generic subkernel proxy factory for kernel-backed capabilities (browser, console, cua)
    function makeSubkernelProxy(rootName, directCaller) {
        function createTarget(path) {
            const callable = function(...args) {
                if (path.length === 0 && directCaller) {
                    return directCaller(...args);
                }
                const argStr = args.length > 0 ? JSON.stringify(args[0]) : "{}";
                const expr = "return await " + rootName + (path.length > 0 ? "." + path.join(".") : "") + "(" + argStr + ");";
                return bridge.call({ tool: rootName, input: { code: expr } });
            };
            return new Proxy(callable, {
                get(target, prop) {
                    if (typeof prop === "symbol" || prop === "then" || prop === "toJSON" || prop === "constructor") {
                        return undefined;
                    }
                    return createTarget(path.concat(String(prop)));
                }
            });
        }
        return createTarget([]);
    }

    // browser capability
    globalThis.browser = makeSubkernelProxy("browser", function(codeOrOpts) {
        const code = typeof codeOrOpts === "string" ? codeOrOpts : (codeOrOpts && codeOrOpts.code ? codeOrOpts.code : String(codeOrOpts));
        return bridge.call({ tool: "browser", input: { code: code } });
    });

    // console desktop control plane: dynamically proxies any console.<path>() method
    // while preserving standard console.log and console.print intact.
    if (globalThis.console) {
        const originalLog = globalThis.console.log || globalThis.print;
        const consoleSubkernel = makeSubkernelProxy("console", function(codeOrOpts) {
            const code = typeof codeOrOpts === "string" ? codeOrOpts : (codeOrOpts && codeOrOpts.code ? codeOrOpts.code : String(codeOrOpts));
            return bridge.call({ tool: "console", input: { code: code } });
        });
        globalThis.console = new Proxy(consoleSubkernel, {
            get(target, prop) {
                if (prop === "log" || prop === "print") {
                    return originalLog;
                }
                return target[prop];
            }
        });
    }

    // MCP namespace: method bridge installed under __harness.mcp.
    globalThis.mcp = bridge.mcp;

    // Builtin docs: help({query, group}) searches, describe(name) shows one.
    globalThis.help = async function(args) {
        if (typeof args === "string") args = { query: args };
        return await bridge.help(args || {});
    };
    globalThis.describe = async function(name) {
        if (typeof name === "string") name = { name: name };
        return await bridge.describe(name || {});
    };

    // Tool discovery
    globalThis.searchTools = async function(query) {
        return await bridge.searchTools({ query: String(query || "") });
    };

    globalThis.listTools = async function() {
        return await bridge.searchTools({ query: "" });
    };

    globalThis.describeTool = async function(name) {
        return await bridge.describeTool({ name: String(name) });
    };

    // Background bash jobs: bashJob("output"|"kill"|"list", jobId, [opts])
    globalThis.bashJob = async function(action, jobId, opts) {
        const input = typeof action === "object" && action !== null
            ? action
            : Object.assign({ action: String(action) }, jobId ? { jobId: String(jobId) } : {}, opts || {});
        const res = await bridge.call({ tool: "bashJob", input: input });
        if (typeof res === "string") return res;
        if (Array.isArray(res) && res.length > 0 && res[0].text !== undefined) {
            return res[0].text;
        }
        if (res && typeof res.content === "string") return res.content;
        return typeof res === "object" ? JSON.stringify(res) : String(res);
    };

    // Subagent orchestration
    globalThis.spawnContext = async function(args) {
        return await bridge.call({ tool: "subagent", input: args || {} });
    };

    // Session state
    globalThis.store = async function(key, val) {
        return await bridge.store({ key: String(key), value: val });
    };

    globalThis.load = async function(key) {
        return await bridge.load({ key: String(key) });
    };

    // Narration
    globalThis.log = function(...args) {
        if (typeof print === "function") {
            print(...args);
        }
    };

    // Command runner: $(cmd) or tagged template
    function buildCmd(strings, values) {
        if (Array.isArray(strings)) {
            let res = "";
            for (let i = 0; i < strings.length; i++) {
                res += strings[i];
                if (i < values.length) {
                    res += values[i];
                }
            }
            return res;
        }
        return String(strings);
    }

    function parseBashOutput(raw) {
        let exitCode = 0;
        let stdout = "";
        let stderr = "";

        const exitMatch = raw.match(/Exit code:\s*(\d+)/);
        if (exitMatch) {
            exitCode = parseInt(exitMatch[1], 10);
        }

        const stdoutMarker = "\nstdout:\n";
        const stderrMarker = "\nstderr:\n";
        const stdoutIdx = raw.indexOf(stdoutMarker);
        const stderrIdx = raw.indexOf(stderrMarker);

        if (stdoutIdx !== -1) {
            if (stderrIdx !== -1 && stderrIdx > stdoutIdx) {
                stdout = raw.substring(stdoutIdx + stdoutMarker.length, stderrIdx);
            } else {
                stdout = raw.substring(stdoutIdx + stdoutMarker.length);
            }
        } else if (raw.includes("stdout: (empty)")) {
            stdout = "";
        } else {
            stdout = raw;
        }

        if (stderrIdx !== -1) {
            stderr = raw.substring(stderrIdx + stderrMarker.length);
        }

        return { exitCode: exitCode, stdout: stdout.trim(), stderr: stderr.trim() };
    }

    class CommandPromise {
        constructor(cmd, opts) {
            this.cmd = cmd;
            this.opts = opts || {};
            this._noThrow = false;
            this._promise = null;
        }

        nothrow() {
            this._noThrow = true;
            return this;
        }

        async _exec() {
            if (!this._promise) {
                this._promise = (async () => {
                    const input = { command: this.cmd };
                    if (this.opts.cwd) input.cwd = String(this.opts.cwd);
                    if (this.opts.env) input.env = this.opts.env;
                    if (this.opts.timeoutMs) input.timeoutMs = this.opts.timeoutMs;
                    if (this.opts.background) input.background = true;

                    const startTime = Date.now();
                    let res;
                    let threw = false;
                    try {
                        res = await bridge.call({ tool: "bash", input: input });
                    } catch (err) {
                        threw = true;
                        res = err.message || String(err);
                    }
                    const durationMs = Date.now() - startTime;

                    let rawOutput = "";
                    let isError = threw;
                    if (typeof res === "string") {
                        rawOutput = res;
                    } else if (Array.isArray(res) && res.length > 0 && res[0].text !== undefined) {
                        rawOutput = res[0].text;
                    } else if (res && typeof res.content === "string") {
                        rawOutput = res.content;
                        if (res.isError) isError = true;
                    } else if (res && typeof res === "object") {
                        rawOutput = JSON.stringify(res);
                    } else {
                        rawOutput = String(res);
                    }

                    if (this.opts.background) {
                        if (isError) {
                            throw new Error(rawOutput);
                        }
                        return rawOutput;
                    }

                    if (rawOutput.includes("This response is not an exit result.")) {
                        if (!this._noThrow) {
                            throw new Error(rawOutput);
                        }
                        return {
                            code: null,
                            exitCode: null,
                            stdout: rawOutput,
                            stderr: rawOutput,
                            ok: false,
                            backgrounded: true,
                            durationMs: durationMs,
                            text() { return rawOutput; },
                            json() { throw new Error("Command moved to background; no JSON exit output yet."); }
                        };
                    }

                    const parsed = parseBashOutput(rawOutput);
                    let exitCode = parsed.exitCode;
                    let stdout = parsed.stdout;
                    let stderr = parsed.stderr;

                    if (isError && exitCode === 0) {
                        exitCode = 1;
                    }
                    if (exitCode !== 0) {
                        isError = true;
                    }

                    if (isError && !this._noThrow) {
                        const errMsg = stderr || stdout || rawOutput;
                        throw new Error("Command failed (exit " + exitCode + "): " + this.cmd + (errMsg ? "\n" + errMsg : ""));
                    }

                    return {
                        code: exitCode,
                        exitCode: exitCode,
                        stdout: stdout,
                        stderr: stderr,
                        ok: exitCode === 0,
                        durationMs: durationMs,
                        text() { return stdout; },
                        json() { return JSON.parse(stdout); }
                    };
                })();
            }
            return this._promise;
        }

        then(onFulfilled, onRejected) {
            return this._exec().then(onFulfilled, onRejected);
        }

        catch(onRejected) {
            return this._exec().catch(onRejected);
        }

        finally(onFinally) {
            return this._exec().finally(onFinally);
        }

        async text() {
            const res = await this._exec();
            return res.text();
        }

        async json() {
            const res = await this._exec();
            return res.json();
        }
    }

    globalThis.$ = function(first, ...rest) {
        if (typeof first === "object" && !Array.isArray(first) && first !== null) {
            const opts = first;
            return function(strings, ...values) {
                const cmd = buildCmd(strings, values);
                return new CommandPromise(cmd, opts);
            };
        }
        const cmd = buildCmd(first, rest);
        return new CommandPromise(cmd, {});
    };
})();
`
