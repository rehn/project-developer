"use client";
import { useEffect, useState } from "react";
import { api } from "@/lib/api";
import { useProjects } from "@/lib/ProjectContext";

type Repo = { fullName: string; cloneUrl: string; private: boolean; description: string | null };

export default function Projects() {
  const { projects, reload, select } = useProjects();
  const [url, setUrl] = useState("");
  const [repos, setRepos] = useState<Repo[] | null>(null);
  const [busy, setBusy] = useState(false);
  const [msg, setMsg] = useState("");
  const [error, setError] = useState("");
  const [status, setStatus] = useState<Record<number, any>>({});

  async function run<T>(fn: () => Promise<T>) {
    setBusy(true); setError(""); setMsg("");
    try { return await fn(); } catch (e: any) { setError(e.message); } finally { setBusy(false); }
  }
  const add = (repoUrl: string) => run(async () => {
    const r = await api("/projects", { body: { repoUrl } });
    await reload(); select(r.id); setUrl(""); setMsg(`Added ${r.name}`);
  });
  const loadRepos = () => run(async () => setRepos(await api("/github/repos")));

  return (
    <div>
      <h2>Projects</h2>
      <div className="card">
        <h3>Add from git repo</h3>
        <form className="row" onSubmit={(e) => { e.preventDefault(); add(url); }}>
          <input style={{ flex: 1 }} placeholder="https://github.com/owner/repo" value={url} onChange={(e) => setUrl(e.target.value)} />
          <button className="primary" disabled={busy || !url}>{busy ? "Working…" : "Clone"}</button>
        </form>
        <button className="link" onClick={loadRepos} disabled={busy}>Or pick from my GitHub repos</button>
        {repos && (
          <ul className="list scroll">
            {repos.map((r) => (
              <li key={r.fullName}>
                <div><b>{r.fullName}</b> {r.private && <span className="tag">private</span>}<div className="muted">{r.description}</div></div>
                <button onClick={() => add(r.cloneUrl)} disabled={busy}>Add</button>
              </li>
            ))}
          </ul>
        )}
        {error && <div className="error">{error}</div>}
        {msg && <div className="ok">{msg}</div>}
      </div>

      {projects.map((p) => (
        <div className="card" key={p.id}>
          <div className="row between">
            <div><b>{p.name}</b><div className="muted">{p.repoUrl}</div></div>
            <div className="row">
              <button onClick={() => run(async () => setStatus({ ...status, [p.id]: await api(`/projects/${p.id}/status`) }))}>Status</button>
              <button onClick={() => run(async () => { const r = await api(`/projects/${p.id}/pull`, { body: {} }); setMsg(`${p.name}: ${r.output}`); })}>Pull</button>
              <button className="danger" onClick={() => confirm(`Remove ${p.name} and delete its checkout?`) && run(async () => { await api(`/projects/${p.id}`, { method: "DELETE" }); await reload(); })}>Remove</button>
            </div>
          </div>
          {status[p.id] && (
            <div className="muted" style={{ marginTop: 8 }}>
              Branch <b>{status[p.id].branch}</b> · {status[p.id].changes.length} uncommitted change(s)
              <ul>{status[p.id].commits.map((c: any) => <li key={c.hash}><code>{c.hash}</code> {c.subject} — {c.author}, {c.when}</li>)}</ul>
            </div>
          )}
        </div>
      ))}
      {!projects.length && <p className="muted">No projects yet. Add one above.</p>}
    </div>
  );
}
