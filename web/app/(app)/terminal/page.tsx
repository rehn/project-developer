"use client";
import { useCallback, useEffect, useState } from "react";
import { api } from "@/lib/api";
import { useProjects } from "@/lib/ProjectContext";
import TermView from "@/components/TermView";
import ProjectPanel from "@/components/ProjectPanel";

type Auth = { installed: boolean; loggedIn?: boolean; authMethod?: string; expiresAt?: string; bin?: string };

export default function Terminal() {
  const { selected } = useProjects();
  const [auth, setAuth] = useState<Auth | null>(null);
  const [loggingIn, setLoggingIn] = useState(false);
  const [cont, setCont] = useState(false);
  const [nonce, setNonce] = useState(0); // bump to start a fresh terminal view
  const [ended, setEnded] = useState(false);
  const [live, setLive] = useState<string[]>([]);

  const checkAuth = useCallback(() => api("/claude/auth").then(setAuth).catch(() => setAuth({ installed: false })), []);
  const refreshLive = useCallback(() => api("/terminal/sessions").then(setLive).catch(() => {}), []);
  useEffect(() => { checkAuth(); }, [checkAuth]);
  useEffect(() => { refreshLive(); }, [refreshLive, nonce, selected?.id]);
  useEffect(() => { setEnded(false); }, [selected?.id]);

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

  if (!selected) return <p className="muted">Add a project first (Projects tab).</p>;

  const key = `p${selected.id}`;
  const running = live.includes(key);
  async function restart() {
    await api(`/terminal?project=${selected!.id}`, { method: "DELETE" }).catch(() => {});
    setEnded(false); setNonce((n) => n + 1);
  }

  return (
    <div className="project-view">
      <div className="term">
        <div className="row between">
          <h2>Claude — {selected.name} {running && <span className="tag">session running</span>}</h2>
          <div className="row">
            {!running && (
              <label className="row muted" title="Resume the most recent conversation in this project">
                <input type="checkbox" checked={cont} onChange={(e) => setCont(e.target.checked)} /> Continue last conversation
              </label>
            )}
            <span className="muted small">signed in via {auth.authMethod ?? "Claude"}</span>
            <button onClick={restart}>Restart</button>
          </div>
        </div>
        <TermView key={`${key}-${nonce}`} target={String(selected.id)} cont={cont} onExit={() => { setEnded(true); refreshLive(); }} />
        {ended && <div className="row"><span className="muted">Session ended.</span><button className="primary" onClick={restart}>Start new session</button></div>}
      </div>
      <ProjectPanel project={selected} />
    </div>
  );
}
