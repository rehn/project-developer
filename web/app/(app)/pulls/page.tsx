"use client";
import { useEffect, useState } from "react";
import { api } from "@/lib/api";

type Group = { projectId: number; project: string; error?: string; pulls: { number: number; title: string; url: string; author: string; draft: boolean; head: string; base: string; updatedAt: string }[] };

export default function Pulls() {
  const [groups, setGroups] = useState<Group[] | null>(null);
  const [error, setError] = useState("");
  const load = () => { setGroups(null); api("/pulls").then(setGroups).catch((e) => setError(e.message)); };
  useEffect(load, []);
  const total = groups?.reduce((n, g) => n + g.pulls.length, 0) ?? 0;

  return (
    <div>
      <div className="row between"><h2>Open pull requests {groups && <span className="tag">{total}</span>}</h2><button onClick={load}>Refresh</button></div>
      {error && <div className="error">{error}</div>}
      {!groups && !error && <p className="muted">Loading…</p>}
      {groups?.map((g) => (
        <div className="card" key={g.projectId}>
          <h3>{g.project}</h3>
          {g.error && <div className="error">{g.error}</div>}
          {!g.error && !g.pulls.length && <div className="muted">No open PRs 🎉</div>}
          <ul className="list">
            {g.pulls.map((p) => (
              <li key={p.number}>
                <div>
                  <a href={p.url} target="_blank" rel="noreferrer">#{p.number} {p.title}</a> {p.draft && <span className="tag">draft</span>}
                  <div className="muted">{p.author} · {p.head} → {p.base} · updated {new Date(p.updatedAt).toLocaleString()}</div>
                </div>
              </li>
            ))}
          </ul>
        </div>
      ))}
    </div>
  );
}
