"use client";
import { useEffect, useState } from "react";
import { api } from "@/lib/api";

// "" = Claude Code's own default. Any other alias or full model ID can be typed under "Other…".
const MODELS = [
  { id: "", label: "Default" },
  { id: "claude-opus-5-5", label: "Opus 5.5" },
  { id: "claude-sonnet-5-5", label: "Sonnet 5.5" },
  { id: "claude-haiku-5-5", label: "Haiku 5.5" },
  { id: "claude-fable-5-2", label: "Fable 5.2" },
];
const OTHER = "__other";

/** Page footer: picks the model new Claude sessions start with (passed as `claude --model`). */
export default function FooterBar() {
  const [model, setModel] = useState<string | null>(null);
  const [custom, setCustom] = useState<string | null>(null); // non-null while typing an "Other…" model
  const [msg, setMsg] = useState<{ ok: boolean; text: string } | null>(null);

  useEffect(() => { api("/settings").then((s) => setModel(s.model ?? "")).catch(() => setModel("")); }, []);

  async function save(m: string) {
    setMsg(null);
    try {
      const r = await api("/settings/model", { method: "PUT", body: { model: m } });
      setModel(r.model); setCustom(null);
      setMsg({ ok: true, text: "Saved — new sessions use it; restart a running one to switch" });
    } catch (e: any) { setMsg({ ok: false, text: e.message }); }
  }

  const known = MODELS.some((m) => m.id === model);
  return (
    <footer className="footbar">
      <label className="row">
        <span className="muted">Model</span>
        {model === null ? <span className="muted small">…</span> : (
          <select value={custom !== null ? OTHER : model} onChange={(e) => e.target.value === OTHER ? setCustom(known ? "" : model) : save(e.target.value)}>
            {MODELS.map((m) => <option key={m.id} value={m.id}>{m.label}</option>)}
            {!known && <option value={model}>{model}</option>}
            <option value={OTHER}>Other…</option>
          </select>
        )}
      </label>
      {custom !== null && (
        <form className="row" onSubmit={(e) => { e.preventDefault(); save(custom); }}>
          <input autoFocus value={custom} onChange={(e) => setCustom(e.target.value)} placeholder="alias or model ID, e.g. opus" />
          <button className="primary" disabled={!custom.trim()}>Save</button>
          <button type="button" onClick={() => setCustom(null)}>Cancel</button>
        </form>
      )}
      {msg && <span className={`${msg.ok ? "muted" : "error"} small`}>{msg.text}</span>}
    </footer>
  );
}
