"use client";
import { useEffect, useState } from "react";
import { api } from "@/lib/api";

export default function Settings() {
  const [s, setS] = useState<{ githubTokenSet: boolean; githubTokenHint: string } | null>(null);
  const [token, setToken] = useState("");
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
        <p className="muted">Used to clone repos and list pull requests. A fine-grained token with Contents + Pull requests (read) works; classic needs <code>repo</code>. Stored encrypted.</p>
        <p>Status: {s?.githubTokenSet ? <b>set ({s.githubTokenHint})</b> : <span className="muted">not set</span>}</p>
        <form className="row" onSubmit={(e) => { e.preventDefault(); act(async () => { const r = await api("/settings/github-token", { method: "PUT", body: { token } }); setToken(""); return token ? `Saved — authenticated as ${r.login}` : "Token removed"; }); }}>
          <input style={{ flex: 1 }} type="password" autoComplete="off" placeholder="ghp_… / github_pat_…" value={token} onChange={(e) => setToken(e.target.value)} />
          <button className="primary">{token ? "Save" : "Remove token"}</button>
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
