"use client";
import { useCallback, useEffect, useState, type ReactNode } from "react";
import { api } from "@/lib/api";
import TermView from "@/components/TermView";

export type Auth = { installed: boolean; loggedIn?: boolean; authMethod?: string; expiresAt?: string; bin?: string };

/** Renders children once the Claude Code CLI is installed and signed in; otherwise shows the install / sign-in steps. */
export default function ClaudeGate({ children }: { children: (auth: Auth) => ReactNode }) {
  const [auth, setAuth] = useState<Auth | null>(null);
  const [loggingIn, setLoggingIn] = useState(false);
  const [nonce] = useState(0);

  const checkAuth = useCallback(() => api("/claude/auth").then(setAuth).catch(() => setAuth({ installed: false })), []);
  useEffect(() => { checkAuth(); }, [checkAuth]);

  if (!auth) return <p className="muted">Checking Claude Code…</p>;

  if (!auth.installed)
    return (
      <div className="card">
        <h2>Claude Code CLI not found</h2>
        <p>The terminal runs the Claude Code CLI on this machine. Install it from <a href="https://claude.com/claude-code" target="_blank" rel="noreferrer">claude.com/claude-code</a>, or point the backend at it with <code>CLAUDE_BIN=/path/to/claude</code>, then re-check.</p>
        <button onClick={checkAuth}>Re-check</button>
      </div>
    );

  if (!auth.loggedIn)
    return (
      <div className="term">
        <div className="card">
          <h2>Sign in to Claude</h2>
          {!loggingIn ? (
            <>
              <p>You aren&apos;t signed in to Claude Code. Click below — a browser window opens to sign in with your Claude account, and this page continues automatically when you&apos;re done.</p>
              <button className="primary" onClick={() => setLoggingIn(true)}>Sign in with Claude</button>
            </>
          ) : (
            <p className="muted">Follow the steps below. If the browser doesn&apos;t open, click the link shown. If it asks for a code, paste it into the terminal and press Enter.</p>
          )}
        </div>
        {loggingIn && (
          <TermView key={`login${nonce}`} target="login" onExit={async () => {
            await checkAuth(); window.dispatchEvent(new Event("pd:auth-changed")); setLoggingIn(false);
          }} />
        )}
      </div>
    );

  return <>{children(auth)}</>;
}
