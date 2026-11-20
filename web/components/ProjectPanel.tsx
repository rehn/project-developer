"use client";
import { useCallback, useEffect, useState } from "react";
import { api, type Project } from "@/lib/api";
import { useTasks } from "@/lib/useTasks";

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
  merged: { number: number; title: string; url: string; author: string; head: string; mergedAt: string }[];
  runs: { id: number; name: string; title: string; url: string; status: string; event: string; branch: string; startedAt: string; actor: string }[];
  release: { tag: string; name: string; url: string; prerelease: boolean; publishedAt: string } | null;
  tags: { name: string; sha: string; url: string }[];
  errors: Record<string, string>;
};

/** Right-hand panel for the selected project: repo, commit hashes, open PRs and latest release. */
export default function ProjectPanel({ project }: { project: Project }) {
  const [info, setInfo] = useState<Info | null>(null);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const [msg, setMsg] = useState("");
  const [merging, setMerging] = useState<number | null>(null);
  const [prMsg, setPrMsg] = useState<{ ok: boolean; text: string } | null>(null);
  const [tagName, setTagName] = useState("");
  const [tagging, setTagging] = useState(false);
  const [tagMsg, setTagMsg] = useState<{ ok: boolean; text: string } | null>(null);
  const { tasks, reload: reloadTasks } = useTasks();
  const mine = tasks.filter((t) => t.toProject === project.id);
  const [taskMsg, setTaskMsg] = useState<{ ok: boolean; text: string } | null>(null);
  async function decide(t: { id: number; from: string }, approve: boolean) {
    setTaskMsg(null);
    try {
      const r = await api(`/tasks/${t.id}/${approve ? "approve" : "reject"}`, { body: {} });
      setTaskMsg({ ok: true, text: approve ? (r.started ? "Approved — started a session with the task. Open the terminal to follow it." : "Approved — sent to the running session.") : "Rejected" });
    } catch (e: any) { setTaskMsg({ ok: false, text: e.message }); }
    reloadTasks();
  }

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

  async function merge(n: number, title: string) {
    if (!confirm(`Merge #${n} "${title}" into ${info?.repo?.defaultBranch ?? "the base branch"}?`)) return;
    setMerging(n); setPrMsg(null);
    try {
      const r = await api(`/projects/${project.id}/pulls/${n}/merge`, { body: { method: "merge" } });
      setPrMsg({ ok: true, text: `#${n}: ${r.message || "merged"}` });
      await load();
    } catch (e: any) { setPrMsg({ ok: false, text: `#${n}: ${e.message}` }); }
    finally { setMerging(null); }
  }

  // Suggest the next versions from the newest semver tag (v1.2.3 → v1.2.4 / v1.3.0 / v2.0.0).
  const latest = info?.tags?.map((t) => /^(v?)(\d+)\.(\d+)\.(\d+)$/.exec(t.name)).find((m) => m != null) ?? null;
  let bumps: [string, string][] = [["", "v0.1.0"]];
  if (latest) {
    const v = latest[1], x = +latest[2], y = +latest[3], z = +latest[4];
    bumps = [["patch", `${v}${x}.${y}.${z + 1}`], ["minor", `${v}${x}.${y + 1}.0`], ["major", `${v}${x + 1}.0.0`]];
  }
  useEffect(() => { setTagName(""); setTagMsg(null); }, [project.id]);
  const tagValue = tagName || bumps[0][1];

  async function createTag() {
    const branch = info?.repo?.defaultBranch ?? "the default branch";
    if (!confirm(`Create tag ${tagValue} on the latest ${branch}?\n\nPushing a tag can start release workflows.`)) return;
    setTagging(true); setTagMsg(null);
    try {
      const r = await api(`/projects/${project.id}/tags`, { body: { name: tagValue } });
      setTagMsg({ ok: true, text: `Created ${r.name} at ${short(r.sha)}` }); setTagName("");
      await load();
    } catch (e: any) { setTagMsg({ ok: false, text: e.message }); }
    finally { setTagging(false); }
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

          {(mine.length > 0 || taskMsg) && (
            <section>
              <h4>Task requests <span className="tag warn">{mine.length}</span></h4>
              <ul className="list tight">
                {mine.map((t) => (
                  <li key={t.id}><div>
                    <b>From {t.from}</b>
                    <div className="muted small taskbody">{t.body}</div>
                    <div className="row" style={{ marginTop: 6 }}>
                      <button className="primary small-btn" onClick={() => decide(t, true)}>Approve</button>
                      <button className="small-btn" onClick={() => decide(t, false)}>Reject</button>
                    </div>
                  </div></li>
                ))}
              </ul>
              {taskMsg && <div className={`${taskMsg.ok ? "ok" : "error"} small`}>{taskMsg.text}</div>}
            </section>
          )}

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
            <h4>Pull requests {info.project.isGithub && !info.errors.pulls && <span className="tag">{info.pulls.length}</span>}</h4>
            {info.errors.pulls && <div className="error small">{info.errors.pulls}</div>}
            {!info.errors.pulls && !info.pulls.length && <div className="muted small">None</div>}
            <ul className="list tight">
              {info.pulls.map((p) => (
                <li key={p.number}><div>
                  <a href={p.url} target="_blank" rel="noreferrer">#{p.number} {p.title}</a> {p.draft && <span className="tag">draft</span>}
                  <div className="muted small">{p.author} · {p.head} · {ago(p.updatedAt)}</div>
                </div>
                <button className="small-btn" onClick={() => merge(p.number, p.title)} disabled={p.draft || merging !== null}
                  title={p.draft ? "Draft pull requests can't be merged" : `Merge #${p.number}`}>{merging === p.number ? "Merging…" : "Merge"}</button>
                </li>
              ))}
            </ul>
            {prMsg && <div className={`${prMsg.ok ? "ok" : "error"} small`}>{prMsg.text}</div>}

            {info.project.isGithub && (
              <>
                <div className="muted small" style={{ marginTop: 8 }}>Recently merged</div>
                {info.errors.merged && <div className="error small">{info.errors.merged}</div>}
                {!info.errors.merged && !info.merged?.length && <div className="muted small">None</div>}
                <ul className="list tight">
                  {info.merged?.map((p) => (
                    <li key={p.number}><div>
                      <a href={p.url} target="_blank" rel="noreferrer">#{p.number} {p.title}</a> <span className="tag">merged</span>
                      <div className="muted small">{p.author} · {p.head} · {ago(p.mergedAt)}</div>
                    </div></li>
                  ))}
                </ul>
              </>
            )}
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

          {info.project.isGithub && (
            <section>
              <h4>Tags</h4>
              {info.errors.tags && <div className="error small">{info.errors.tags}</div>}
              {!info.errors.tags && !info.tags?.length && <div className="muted small">No tags yet</div>}
              <ul className="list tight">
                {info.tags?.map((t) => (
                  <li key={t.name}>
                    <a href={t.url} target="_blank" rel="noreferrer"><b>{t.name}</b></a>
                    <span className="row"><code className="muted">{short(t.sha)}</code>
                      {info.commit && t.sha === info.commit.hash && <span className="tag">{info.repo?.defaultBranch}</span>}</span>
                  </li>
                ))}
              </ul>
              <div className="tagform">
                <div className="muted small">New tag on latest {info.repo?.defaultBranch ?? "default branch"}{info.commit && <> (<code>{short(info.commit.hash)}</code>)</>}</div>
                <div className="row">
                  <input value={tagValue} onChange={(e) => setTagName(e.target.value)} onKeyDown={(e) => e.key === "Enter" && !tagging && createTag()} aria-label="Tag name" />
                  <button className="primary" onClick={createTag} disabled={tagging || !tagValue.trim()}>{tagging ? "Creating…" : "Create tag"}</button>
                </div>
                {latest && (
                  <div className="row">
                    {bumps.map(([k, v]) => <button key={k} className={`small-btn ${tagValue === v ? "on" : ""}`} onClick={() => setTagName(v)}>{k} {v}</button>)}
                  </div>
                )}
                {tagMsg && <div className={`${tagMsg.ok ? "ok" : "error"} small`}>{tagMsg.text}</div>}
              </div>
            </section>
          )}
        </>
      )}
    </div>
  );
}
