"use client";
import { useCallback, useEffect, useState } from "react";
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

  // Keep each project's git status fresh: load on mount, then every 30s while the tab is visible.
  const [checkedAt, setCheckedAt] = useState<Date | null>(null);
  const refreshStatus = useCallback(async () => {
    if (!projects.length) return;
    const entries = await Promise.all(projects.map(async (p) => {
      try { return [p.id, await api(`/projects/${p.id}/status`)] as const; }
      catch (e: any) { return [p.id, { error: e.message }] as const; }
    }));
    setStatus(Object.fromEntries(entries));
    setCheckedAt(new Date());
  }, [projects]);
  useEffect(() => {
    refreshStatus();
    const t = setInterval(() => { if (!document.hidden) refreshStatus(); }, 30_000);
    const onVisible = () => { if (!document.hidden) refreshStatus(); };
    document.addEventListener("visibilitychange", onVisible);
    return () => { clearInterval(t); document.removeEventListener("visibilitychange", onVisible); };
  }, [refreshStatus]);

  return (
    <div>
      <div className="row between">
        <h2>Projects</h2>
        {checkedAt && <span className="muted small">Status updated {checkedAt.toLocaleTimeString()}</span>}
      </div>
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
              <button onClick={() => run(refreshStatus)} title="Refresh now (updates automatically every 30s)">Refresh</button>
              <button onClick={() => run(async () => { const r = await api(`/projects/${p.id}/pull`, { body: {} }); setMsg(`${p.name}: ${r.output}`); await refreshStatus(); })}>Pull</button>
              <button className="danger" onClick={() => confirm(`Remove ${p.name} and delete its checkout?`) && run(async () => { await api(`/projects/${p.id}`, { method: "DELETE" }); await reload(); })}>Remove</button>
            </div>
          </div>
          {status[p.id]?.error && <div className="error" style={{ marginTop: 8 }}>{status[p.id].error}</div>}
          {status[p.id] && !status[p.id].error && (
            <div className="muted" style={{ marginTop: 8 }}>
              Branch <b>{status[p.id].branch}</b> · {status[p.id].changes.length} uncommitted change(s)
              {status[p.id].behind > 0 && <> · <span className="tag">{status[p.id].behind} new on GitHub — Pull to update</span></>}
              {status[p.id].ahead > 0 && <> · {status[p.id].ahead} not pushed</>}
              {status[p.id].behind === 0 && status[p.id].ahead === 0 && <> · up to date with GitHub</>}
              {status[p.id].fetchError && <div className="error">Couldn&apos;t check GitHub: {status[p.id].fetchError}</div>}
              <ul>{status[p.id].commits.map((c: any) => <li key={c.hash}><code>{c.hash}</code> {c.subject} — {c.author}, {c.when}</li>)}</ul>
            </div>
          )}
        </div>
      ))}
      {!projects.length && <p className="muted">No projects yet. Add one above.</p>}
    </div>
  );
}
