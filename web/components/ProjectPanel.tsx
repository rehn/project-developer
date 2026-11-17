"use client";
import { useCallback, useEffect, useState } from "react";
import { api, type Project } from "@/lib/api";

function ago(iso?: string | null) {
  if (!iso) return "";
  const s = (Date.now() - Date.parse(iso)) / 1000;
  if (!(s >= 0)) return "just now";
  const u: [number, string][] = [[86400 * 30, "mo"], [86400, "d"], [3600, "h"], [60, "m"]];
  for (const [n, l] of u) if (s >= n) return `${Math.floor(s / n)}${l} ago`;
  return "just now";
}
const short = (h?: string) => (h ? h.slice(0, 7) : "");

type Info = {
  project: { isGithub: boolean; repoUrl: string };
  local: { hash?: string; branch?: string; changes?: number };
  repo: { fullName: string; url: string; description: string | null; defaultBranch: string; private: boolean } | null;
  commit: { hash: string; url: string; message: string; author: string; date: string } | null;
  pulls: { number: number; title: string; url: string; author: string; draft: boolean; head: string; updatedAt: string }[];
  runs: { id: number; name: string; title: string; url: string; status: string; event: string; branch: string; startedAt: string; actor: string }[];
  release: { tag: string; name: string; url: string; prerelease: boolean; publishedAt: string } | null;
  errors: Record<string, string>;
};

/** Right-hand panel for the selected project: repo, commit hashes, open PRs and latest release. */
export default function ProjectPanel({ project }: { project: Project }) {
  const [info, setInfo] = useState<Info | null>(null);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const [msg, setMsg] = useState("");

  const load = useCallback(() => {
    setBusy(true);
    return api(`/projects/${project.id}/info`)
      .then((d) => { setInfo(d); setError(""); })
      .catch((e) => setError(e.message))
      .finally(() => setBusy(false));
  }, [project.id]);

  useEffect(() => {
    setInfo(null); setMsg("");
    load();
  }, [load]);

  // Poll faster while workflow runs are active so they disappear soon after finishing.
  const active = !!info?.runs?.length;
  useEffect(() => {
    const t = setInterval(load, active ? 15_000 : 60_000);
    return () => clearInterval(t);
  }, [load, active]);

  async function pull() {
    setBusy(true); setMsg("");
    try { const r = await api(`/projects/${project.id}/pull`, { body: {} }); setMsg(r.output || "Pulled"); await load(); }
    catch (e: any) { setMsg(e.message); setBusy(false); }
  }

  const behind = info?.local.hash && info.commit && info.local.hash !== info.commit.hash
    && info.local.branch === info.repo?.defaultBranch;

  return (
    <div className="panel">
      <div className="row between">
        <h3>Repository</h3>
        <button onClick={load} disabled={busy} title="Refresh">{busy ? "…" : "↻"}</button>
      </div>
      {error && <div className="error">{error}</div>}
      {!info && !error && <p className="muted">Loading…</p>}

      {info && (
        <>
          <section>
            {info.repo
              ? <><a href={info.repo.url} target="_blank" rel="noreferrer"><b>{info.repo.fullName}</b></a> {info.repo.private && <span className="tag">private</span>}
                  {info.repo.description && <div className="muted">{info.repo.description}</div>}</>
              : <div className="muted">{info.project.isGithub ? "GitHub details unavailable" : "Not a GitHub repository"}</div>}
          </section>

          <section>
            <h4>Commit</h4>
            <div className="kv"><span className="muted">Local</span>
              <span><code>{short(info.local.hash) || "?"}</code> {info.local.branch && <span className="tag">{info.local.branch}</span>}
                {!!info.local.changes && <span className="tag warn">{info.local.changes} uncommitted</span>}</span></div>
            {info.commit && (
              <div className="kv"><span className="muted">Remote</span>
                <span><a href={info.commit.url} target="_blank" rel="noreferrer"><code>{short(info.commit.hash)}</code></a> <span className="tag">{info.repo?.defaultBranch}</span></span></div>
            )}
            {info.commit && <div className="muted small">{info.commit.message} — {info.commit.author}, {ago(info.commit.date)}</div>}
            {behind && <div className="row" style={{ marginTop: 6 }}><span className="muted small">Local differs from remote.</span><button onClick={pull} disabled={busy}>Pull</button></div>}
            {msg && <div className="muted small">{msg}</div>}
            {info.errors.commit && <div className="error small">{info.errors.commit}</div>}
          </section>

          {info.project.isGithub && (
            <section>
              <h4>Running actions {!info.errors.runs && <span className="tag">{info.runs?.length ?? 0}</span>}</h4>
              {info.errors.runs && <div className="error small">{info.errors.runs}</div>}
              {!info.errors.runs && !info.runs?.length && <div className="muted small">Nothing running</div>}
              <ul className="list tight">
                {info.runs?.map((r) => (
                  <li key={r.id}><div className="run">
                    <span className={`run-dot ${r.status}`} title={r.status.replace("_", " ")} />
                    <div>
                      <a href={r.url} target="_blank" rel="noreferrer">{r.name}</a> {r.status !== "in_progress" && <span className="tag warn">{r.status.replace("_", " ")}</span>}
                      <div className="muted small">{r.title}</div>
                      <div className="muted small">{r.branch} · {r.event} · {r.actor}{r.startedAt && ` · ${ago(r.startedAt)}`}</div>
                    </div>
                  </div></li>
                ))}
              </ul>
            </section>
          )}

          <section>
            <h4>Open pull requests {info.project.isGithub && !info.errors.pulls && <span className="tag">{info.pulls.length}</span>}</h4>
            {info.errors.pulls && <div className="error small">{info.errors.pulls}</div>}
            {!info.errors.pulls && !info.pulls.length && <div className="muted small">None</div>}
            <ul className="list tight">
              {info.pulls.map((p) => (
                <li key={p.number}><div>
                  <a href={p.url} target="_blank" rel="noreferrer">#{p.number} {p.title}</a> {p.draft && <span className="tag">draft</span>}
                  <div className="muted small">{p.author} · {p.head} · {ago(p.updatedAt)}</div>
                </div></li>
              ))}
            </ul>
          </section>

          <section>
            <h4>Latest release</h4>
            {info.errors.release && <div className="error small">{info.errors.release}</div>}
            {!info.errors.release && !info.release && info.project.isGithub && <div className="muted small">No releases yet</div>}
            {info.release && (
              <div>
                <a href={info.release.url} target="_blank" rel="noreferrer"><b>{info.release.tag}</b></a> {info.release.prerelease && <span className="tag">pre-release</span>}
                <div className="muted small">{ago(info.release.publishedAt)}</div>
              </div>
            )}
          </section>
        </>
      )}
    </div>
  );
}
