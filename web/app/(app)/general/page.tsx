"use client";
import { useCallback, useEffect, useState } from "react";
import { api } from "@/lib/api";
import ClaudeGate, { type Auth } from "@/components/ClaudeGate";
import TermView from "@/components/TermView";

export default function General() {
  return <ClaudeGate>{(auth) => <GeneralTerminal auth={auth} />}</ClaudeGate>;
}

/** A Claude session that isn't tied to any project (runs in its own scratch directory). */
function GeneralTerminal({ auth }: { auth: Auth }) {
  const [cont, setCont] = useState(false);
  const [nonce, setNonce] = useState(0);
  const [ended, setEnded] = useState(false);
  const [running, setRunning] = useState(false);

  const refreshLive = useCallback(() => api("/terminal/sessions").then((l: string[]) => setRunning(l.includes("general"))).catch(() => {}), []);
  useEffect(() => { refreshLive(); }, [refreshLive, nonce]);

  async function restart() {
    await api("/terminal?project=general", { method: "DELETE" }).catch(() => {});
    setEnded(false); setNonce((n) => n + 1);
  }

  return (
    <div className="term">
      <div className="row between">
        <h2>Claude — General {running && <span className="tag">session running</span>}</h2>
        <div className="row">
          {!running && (
            <label className="row muted" title="Resume the most recent general conversation">
              <input type="checkbox" checked={cont} onChange={(e) => setCont(e.target.checked)} /> Continue last conversation
            </label>
          )}
          <span className="muted small">signed in via {auth.authMethod ?? "Claude"}</span>
          <button onClick={restart}>Restart</button>
        </div>
      </div>
      <TermView key={`general-${nonce}`} target="general" cont={cont} onExit={() => { setEnded(true); refreshLive(); }} />
      {ended && <div className="row"><span className="muted">Session ended.</span><button className="primary" onClick={restart}>Start new session</button></div>}
    </div>
  );
}
