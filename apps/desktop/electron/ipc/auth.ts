import http from "node:http";
import { ipcMain, shell, BrowserWindow } from "electron";

interface OAuthCallbackServerResult {
  code: string;
}

function startCallbackServer(port: number, callbackPath: string): Promise<OAuthCallbackServerResult> {
  return new Promise((resolve, reject) => {
    const server = http.createServer((req, res) => {
      try {
        const reqUrl = new URL(req.url || "/", `http://localhost:${port}`);
        if (reqUrl.pathname === callbackPath) {
          const code = reqUrl.searchParams.get("code");
          const error = reqUrl.searchParams.get("error");

          res.writeHead(200, { "Content-Type": "text/html; charset=utf-8" });
          res.end(`
            <!DOCTYPE html>
            <html>
              <head><title>Console Auth</title></head>
              <body style="background:#000;color:#fff;font-family:-apple-system,BlinkMacSystemFont,sans-serif;display:flex;align-items:center;justify-content:center;height:100vh;margin:0;">
                <div style="text-align:center;background:#121212;padding:36px;border-radius:12px;border:1px solid #222;max-width:380px;">
                  <h2 style="margin:0 0 8px 0;font-size:18px;font-weight:600;">Authentication Successful</h2>
                  <p style="margin:0;font-size:13px;color:#888;">You can now close this tab and return to Console.</p>
                </div>
              </body>
            </html>
          `);

          server.close();
          if (error) {
            reject(new Error(`OAuth failed: ${error}`));
          } else if (code) {
            resolve({ code });
          } else {
            reject(new Error("No code parameter received in OAuth callback."));
          }
        } else {
          res.writeHead(404);
          res.end();
        }
      } catch (err) {
        res.writeHead(500);
        res.end();
        server.close();
        reject(err);
      }
    });

    server.on("error", (err) => {
      server.close();
      reject(err);
    });

    // Timeout after 2 minutes
    const timeout = setTimeout(() => {
      server.close();
      reject(new Error("OAuth login timed out."));
    }, 120_000);

    server.listen(port, "127.0.0.1", () => {
      server.unref();
    });

    server.on("close", () => {
      clearTimeout(timeout);
    });
  });
}

export function registerAuthIpc(getMainWindow: () => BrowserWindow | null): void {
  ipcMain.handle(
    "auth:loginWithBrowser",
    async (_event, { provider, authUrl, port, callbackPath }: {
      provider: string;
      authUrl: string;
      port?: number;
      callbackPath?: string;
    }) => {
      const listenPort = port || (provider === "antigravity" ? 8086 : 8085);
      const listenPath = callbackPath || "/oauth2callback";

      // Start the local callback server in background
      const serverPromise = startCallbackServer(listenPort, listenPath);

      // Open browser
      await shell.openExternal(authUrl);

      // Wait for OAuth redirect
      const { code } = await serverPromise;

      // Bring desktop app back to focus
      const win = getMainWindow();
      if (win) {
        win.show();
        win.focus();
      }

      return { code };
    },
  );
}
