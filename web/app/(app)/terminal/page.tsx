"use client";
import { useCallback, useEffect, useState } from "react";
import Link from "next/link";
import { api } from "@/lib/api";
import { useProjects } from "@/lib/ProjectContext";
import ClaudeGate, { type Auth } from "@/components/ClaudeGate";
import TermView from "@/components/TermView";
import ProjectPanel from "@/components/ProjectPanel";
import UploadButtons from "@/components/UploadButtons";
import EditorView from "@/components/EditorView";

export default function Terminal() {
  return <ClaudeGate>{(auth) => <ProjectTerminal auth={auth} />}</ClaudeGate>;
}

function ProjectTerminal({ auth }: { auth: Auth }) {
  const { selected } = useProjects();
  const [cont, setCont] = useState(false);
  const [nonce, setNonce] = useState(0); // bump to start a fresh terminal view
  const [ended, setEnded] = useState(false);
  const [live, setLive] = useState<string[]>([]);
  const [mode, setMode] = useState<"claude" | "editor">("claude");

  const refreshLive = useCallback(() => api("/terminal/sessions").then(setLive).catch(() => {}), []);
  useEffect(() => { refreshLive(); }, [refreshLive, nonce, selected?.id]);
  useEffect(() => { setEnded(false); }, [selected?.id]);

  if (!selected) return <p className="muted">No project yet — <Link href="/projects">add one</Link> (or use + in the tab bar).</p>;

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
          <h2>{mode === "claude" ? "Claude" : "Editor"} — {selected.name} {mode === "claude" && running && <span className="tag">session running</span>}</h2>
          <div className="row">
            <UploadButtons project={selected} />
            <div className="mode-switch" role="tablist" aria-label="View">
              <button role="tab" aria-selected={mode === "claude"} className={mode === "claude" ? "on" : ""} onClick={() => setMode("claude")}>Claude</button>
              <button role="tab" aria-selected={mode === "editor"} className={mode === "editor" ? "on" : ""} onClick={() => setMode("editor")}>Editor</button>
            </div>
            {mode === "claude" && <>
            {!running && (
              <label className="row muted" title="Resume the most recent conversation in this project">
                <input type="checkbox" checked={cont} onChange={(e) => setCont(e.target.checked)} /> Continue last conversation
              </label>
            )}
            <span className="muted small">signed in via {auth.authMethod ?? "Claude"}</span>
            <button onClick={restart}>Restart</button>
            </>}
          </div>
        </div>
        {mode === "editor" ? <EditorView key={selected.id} project={selected} /> : <>
        <TermView key={`${key}-${nonce}`} target={String(selected.id)} cont={cont} onExit={() => { setEnded(true); refreshLive(); }} />
        {ended && <div className="row"><span className="muted">Session ended.</span><button className="primary" onClick={restart}>Start new session</button></div>}
        </>}
      </div>
      <ProjectPanel key={selected.id} project={selected} />
    </div>
  );
}
