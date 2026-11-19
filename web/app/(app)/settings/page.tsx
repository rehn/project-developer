"use client";
import { useEffect, useState } from "react";
import { api } from "@/lib/api";

export default function Settings() {
  const [s, setS] = useState<{ githubTokenSet: boolean; githubTokenHint: string; kubeconfigSet: boolean; kubeContexts: string[]; kubeCurrent: string } | null>(null);
  const [token, setToken] = useState("");
  const [kube, setKube] = useState("");
  const [cur, setCur] = useState("");
  const [next, setNext] = useState("");
  const [msg, setMsg] = useState("");
  const [error, setError] = useState("");
  const load = () => api("/settings").then(setS);
  useEffect(() => { load(); }, []);

  async function act(fn: () => Promise<string>) {
    setMsg(""); setError("");
    try { setMsg(await fn()); await load(); } catch (e: any) { setError(e.message); }
  }

  return (
    <div>
      <h2>Settings</h2>
      {msg && <div className="ok">{msg}</div>}
      {error && <div className="error">{error}</div>}
      <div className="card">
        <h3>GitHub access token</h3>
        <p className="muted">Used to clone repos, show repository info, merge pull requests and create tags. A fine-grained token needs Contents and Pull requests (read and write) plus Actions (read); classic needs <code>repo</code>. Stored encrypted.</p>
        <p>Status: {s?.githubTokenSet ? <b>set ({s.githubTokenHint})</b> : <span className="muted">not set</span>}</p>
        <form className="row" onSubmit={(e) => { e.preventDefault(); act(async () => { const r = await api("/settings/github-token", { method: "PUT", body: { token } }); setToken(""); return token ? `Saved — authenticated as ${r.login}` : "Token removed"; }); }}>
          <input style={{ flex: 1 }} type="password" autoComplete="off" placeholder="ghp_… / github_pat_…" value={token} onChange={(e) => setToken(e.target.value)} />
          <button className="primary">{token ? "Save" : "Remove token"}</button>
        </form>
      </div>
      <div className="card">
        <h3>Kubernetes access (kubeconfig)</h3>
        <p className="muted">Lets <code>kubectl</code> work inside Claude sessions. Works for any cluster the server can reach, including the one this app runs in. Stored on the data volume (not encrypted at rest) and exposed to sessions as <code>$KUBECONFIG</code>; running sessions need a restart to pick it up.</p>
        <p>Status: {s?.kubeconfigSet ? <b>set{s.kubeCurrent ? ` (current context: ${s.kubeCurrent})` : ""}</b> : <span className="muted">not set</span>}</p>
        {!!s?.kubeContexts?.length && <p className="muted small">Contexts: {s.kubeContexts.join(", ")}</p>}
        <form onSubmit={(e) => { e.preventDefault(); act(async () => { await api("/settings/kubeconfig", { method: "PUT", body: { content: kube } }); setKube(""); return kube.trim() ? "Kubeconfig saved" : "Kubeconfig removed"; }); }}>
          <textarea style={{ width: "100%", minHeight: 120, fontFamily: "ui-monospace, monospace" }} placeholder="Paste a kubeconfig here, or choose a file…" value={kube} onChange={(e) => setKube(e.target.value)} />
          <div className="row">
            <input type="file" onChange={async (e) => { const f = e.target.files?.[0]; if (f) setKube(await f.text()); e.target.value = ""; }} />
            <button className="primary">{kube.trim() ? "Save" : "Remove kubeconfig"}</button>
          </div>
        </form>
      </div>
      <div className="card">
        <h3>Change password</h3>
        <form className="row" onSubmit={(e) => { e.preventDefault(); act(async () => { await api("/auth/password", { body: { current: cur, next } }); setCur(""); setNext(""); return "Password changed"; }); }}>
          <input type="password" placeholder="Current" autoComplete="current-password" value={cur} onChange={(e) => setCur(e.target.value)} />
          <input type="password" placeholder="New (min 8)" autoComplete="new-password" value={next} onChange={(e) => setNext(e.target.value)} />
          <button className="primary">Change</button>
        </form>
      </div>
    </div>
  );
}
