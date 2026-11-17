"use client";
import { useEffect, useRef, useState } from "react";
import { useProjects } from "@/lib/ProjectContext";

type Line = { kind: "user" | "text" | "tool" | "result" | "error" | "info"; text: string };

function toLines(ev: any): Line[] {
  if (ev.type === "assistant") {
    return (ev.message?.content ?? []).flatMap((b: any): Line[] => {
      if (b.type === "text" && b.text?.trim()) return [{ kind: "text", text: b.text }];
      if (b.type === "tool_use") {
        const i = b.input ?? {};
        const detail = i.command ?? i.file_path ?? i.pattern ?? i.path ?? i.description ?? JSON.stringify(i).slice(0, 120);
        return [{ kind: "tool", text: `${b.name}  ${String(detail).slice(0, 200)}` }];
      }
      return [];
    });
  }
  if (ev.type === "result") {
    const cost = typeof ev.total_cost_usd === "number" ? ` · $${ev.total_cost_usd.toFixed(4)}` : "";
    return [{ kind: ev.is_error ? "error" : "result", text: `${ev.is_error ? "Failed" : "Done"} in ${Math.round((ev.duration_ms ?? 0) / 1000)}s${cost}` }];
  }
  if (ev.type === "app_error") return [{ kind: "error", text: ev.error }];
  return [];
}

export default function Terminal() {
  const { selected } = useProjects();
  const [logs, setLogs] = useState<Record<number, Line[]>>({});
  const [sessions, setSessions] = useState<Record<number, string>>({});
  const [prompt, setPrompt] = useState("");
  const [mode, setMode] = useState("acceptEdits");
  const [busy, setBusy] = useState(false);
  const abort = useRef<AbortController | null>(null);
  const end = useRef<HTMLDivElement>(null);
  const pid = selected?.id;
  const lines = (pid && logs[pid]) || [];
  useEffect(() => { end.current?.scrollIntoView({ block: "end" }); }, [lines.length]);

  const push = (id: number, ...l: Line[]) => setLogs((p) => ({ ...p, [id]: [...(p[id] ?? []), ...l] }));

  async function send() {
    if (!pid || !prompt.trim() || busy) return;
    const id = pid, text = prompt;
    setPrompt(""); setBusy(true);
    push(id, { kind: "user", text });
    const ctl = (abort.current = new AbortController());
    try {
      const res = await fetch("/api/claude/run", {
        method: "POST", headers: { "Content-Type": "application/json" }, signal: ctl.signal,
        body: JSON.stringify({ projectId: id, prompt: text, mode, sessionId: sessions[id] }),
      });
      if (!res.ok || !res.body) throw new Error((await res.json().catch(() => ({}))).error ?? `HTTP ${res.status}`);
      const reader = res.body.getReader(), dec = new TextDecoder();
      let buf = "";
      for (;;) {
        const { done, value } = await reader.read();
        if (done) break;
        buf += dec.decode(value, { stream: true });
        let nl: number;
        while ((nl = buf.indexOf("\n")) >= 0) {
          const raw = buf.slice(0, nl); buf = buf.slice(nl + 1);
          if (!raw.trim()) continue;
          try {
            const ev = JSON.parse(raw);
            if (ev.session_id) setSessions((s) => ({ ...s, [id]: ev.session_id }));
            const l = toLines(ev);
            if (l.length) push(id, ...l);
          } catch { /* ignore partial/non-JSON line */ }
        }
      }
    } catch (e: any) {
      push(id, e.name === "AbortError" ? { kind: "info", text: "Stopped." } : { kind: "error", text: e.message });
    } finally { setBusy(false); abort.current = null; }
  }

  if (!selected) return <p className="muted">Add a project first (Projects tab).</p>;
  return (
    <div className="term">
      <div className="row between">
        <h2>Terminal — {selected.name}</h2>
        <div className="row">
          <select value={mode} onChange={(e) => setMode(e.target.value)} disabled={busy}>
            <option value="plan">Plan only (read-only)</option>
            <option value="acceptEdits">Auto-accept edits</option>
            <option value="bypassPermissions">Bypass permissions ⚠</option>
          </select>
          <button onClick={() => { setSessions((s) => { const n = { ...s }; delete n[selected.id]; return n; }); push(selected.id, { kind: "info", text: "— new session —" }); }} disabled={busy}>New session</button>
        </div>
      </div>
      <div className="screen">
        {!lines.length && <div className="muted">Describe a feature or change to implement in {selected.name}.</div>}
        {lines.map((l, i) => <div key={i} className={`ln ${l.kind}`}>{l.kind === "user" ? "> " : l.kind === "tool" ? "⏺ " : ""}{l.text}</div>)}
        {busy && <div className="ln info">Claude is working…</div>}
        <div ref={end} />
      </div>
      <form className="row" onSubmit={(e) => { e.preventDefault(); send(); }}>
        <textarea rows={3} style={{ flex: 1 }} placeholder="Implement…  (Enter to send, Shift+Enter for newline)" value={prompt}
          onChange={(e) => setPrompt(e.target.value)} onKeyDown={(e) => { if (e.key === "Enter" && !e.shiftKey) { e.preventDefault(); send(); } }} />
        {busy ? <button type="button" className="danger" onClick={() => abort.current?.abort()}>Stop</button> : <button className="primary" disabled={!prompt.trim()}>Send</button>}
      </form>
    </div>
  );
}
