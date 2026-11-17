"use client";
import { useEffect, useState } from "react";
import { api, fmt } from "@/lib/api";

type T = { input: number; output: number; cacheWrite: number; cacheRead: number; messages: number };
const sum = (t: T) => t.input + t.output + t.cacheWrite + t.cacheRead;


export default function Usage() {
  const [u, setU] = useState<any>(null);
  const [error, setError] = useState("");
  useEffect(() => { api("/usage").then(setU).catch((e) => setError(e.message)); }, []);
  if (error) return <div className="error">{error}</div>;
  if (!u) return <p className="muted">Loading…</p>;

  const days: (T & { date: string })[] = u.byDay.slice(-30);
  const max = Math.max(1, ...days.map((d) => d.input + d.output + d.cacheWrite));
  const Tile = ({ label, t }: { label: string; t: T }) => (
    <div className="card tile">
      <div className="muted">{label}</div>
      <div className="big">{fmt(t.output)}<span className="muted"> out</span></div>
      <div className="muted">{fmt(t.input + t.cacheWrite)} in · {fmt(t.cacheRead)} cached · {t.messages} msgs</div>
    </div>
  );
  return (
    <div>
      <h2>Claude usage</h2>
      <p className="muted">Tokens aggregated from Claude Code session logs on this machine (~/.claude/projects). Cached reads are shown separately.</p>
      <div className="tiles"><Tile label="Last 5 hours" t={u.last5h} /><Tile label="Last 7 days" t={u.last7d} /><Tile label="All time" t={u.total} /></div>

      <div className="card">
        <h3>Daily tokens (last 30 days, excl. cache reads)</h3>
        <div className="bars">
          {days.map((d) => (
            <div key={d.date} className="bar" title={`${d.date}: ${fmt(d.input + d.cacheWrite)} in, ${fmt(d.output)} out`}>
              <div className="in" style={{ height: `${((d.input + d.cacheWrite) / max) * 100}%` }} />
              <div className="out" style={{ height: `${(d.output / max) * 100}%` }} />
            </div>
          ))}
        </div>
        <div className="muted">Blue = input + cache writes, green = output</div>
      </div>

      <div className="card">
        <h3>By model</h3>
        <table><thead><tr><th>Model</th><th>Input</th><th>Output</th><th>Cache read</th><th>Msgs</th></tr></thead>
          <tbody>{u.byModel.map((m: any) => <tr key={m.model}><td>{m.model}</td><td>{fmt(m.input + m.cacheWrite)}</td><td>{fmt(m.output)}</td><td>{fmt(m.cacheRead)}</td><td>{m.messages}</td></tr>)}</tbody></table>
      </div>
      <div className="card">
        <h3>By project</h3>
        <table><thead><tr><th>Project</th><th>Output</th><th>Total</th></tr></thead>
          <tbody>{u.byProject.map((p: any) => <tr key={p.project}><td>{p.project}</td><td>{fmt(p.output)}</td><td>{fmt(sum(p))}</td></tr>)}</tbody></table>
      </div>
    </div>
  );
}
