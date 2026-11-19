"use client";
import { useCallback, useEffect, useState } from "react";
import Link from "next/link";
import { api } from "@/lib/api";
import { useProjects } from "@/lib/ProjectContext";
import ClaudeGate, { type Auth } from "@/components/ClaudeGate";
import TermView from "@/components/TermView";
import ProjectPanel from "@/components/ProjectPanel";

export default function Terminal() {
  return <ClaudeGate>{(auth) => <ProjectTerminal auth={auth} />}</ClaudeGate>;
}

function ProjectTerminal({ auth }: { auth: Auth }) {
  const { selected } = useProjects();
  const [cont, setCont] = useState(false);
  const [nonce, setNonce] = useState(0); // bump to start a fresh terminal view
  const [ended, setEnded] = useState(false);
  const [live, setLive] = useState<string[]>([]);

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
      <ProjectPanel key={selected.id} project={selected} />
    </div>
  );
}
