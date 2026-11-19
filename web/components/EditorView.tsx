"use client";
import { useCallback, useEffect, useRef, useState } from "react";
import { api, type Project } from "@/lib/api";
import TermView from "@/components/TermView";

// Monaco (the VS Code editor) is loaded in the browser from a CDN, so the app needs no extra npm package.
const MONACO = "https://cdn.jsdelivr.net/npm/monaco-editor@0.52.2/min";

let monacoP: Promise<any> | null = null;
function loadMonaco(): Promise<any> {
  if (monacoP) return monacoP;
  monacoP = new Promise((resolve, reject) => {
    const w = window as any;
    // Workers can't be loaded cross-origin directly; a tiny same-origin blob imports them.
    w.MonacoEnvironment = {
      getWorkerUrl: () => URL.createObjectURL(new Blob(
        [`self.MonacoEnvironment={baseUrl:'${MONACO}/'};importScripts('${MONACO}/vs/base/worker/workerMain.js');`],
        { type: "text/javascript" })),
    };
    const s = document.createElement("script");
    s.src = `${MONACO}/vs/loader.js`;
    s.onload = () => {
      w.require.config({ paths: { vs: `${MONACO}/vs` } });
      w.require(["vs/editor/editor.main"], () => resolve(w.monaco), reject);
    };
    s.onerror = () => { monacoP = null; reject(new Error("Couldn't load the editor from the CDN (is the browser online?)")); };
    document.head.appendChild(s);
  });
  return monacoP;
}

type Entry = { name: string; dir: boolean };
const join = (a: string, b: string) => (a ? `${a}/${b}` : b);
const base = (p: string) => p.slice(p.lastIndexOf("/") + 1);

/** File tree + Monaco editor on top, a plain shell in the project checkout below. */
export default function EditorView({ project }: { project: Project }) {
  const host = useRef<HTMLDivElement>(null);
  const editor = useRef<any>(null);
  const monaco = useRef<any>(null);
  const models = useRef(new Map<string, any>());   // path -> Monaco model
  const saved = useRef(new Map<string, string>());  // path -> last saved content
  const [tree, setTree] = useState<Record<string, Entry[]>>({});
  const [open, setOpen] = useState<Record<string, boolean>>({ "": true });
  const [tabs, setTabs] = useState<string[]>([]);
  const [active, setActive] = useState("");
  const [dirty, setDirty] = useState<Record<string, boolean>>({});
  const [msg, setMsg] = useState<{ ok: boolean; text: string } | null>(null);
  const [ready, setReady] = useState(false);
  const [shellN, setShellN] = useState(0);
  const [shellEnded, setShellEnded] = useState(false);

  const loadDir = useCallback(async (dir: string) => {
    try { const l = await api(`/projects/${project.id}/files?path=${encodeURIComponent(dir)}`); setTree((t) => ({ ...t, [dir]: l })); }
    catch (e: any) { setMsg({ ok: false, text: e.message }); }
  }, [project.id]);
  useEffect(() => { loadDir(""); }, [loadDir]);

  // Create the editor once Monaco is available.
  const saveRef = useRef<() => void>(() => {});
  useEffect(() => {
    let dead = false;
    loadMonaco().then((m) => {
      if (dead || !host.current) return;
      monaco.current = m;
      editor.current = m.editor.create(host.current, { theme: "vs-dark", automaticLayout: true, minimap: { enabled: false }, fontSize: 13, scrollBeyondLastLine: false });
      editor.current.addCommand(m.KeyMod.CtrlCmd | m.KeyCode.KeyS, () => saveRef.current());
      setReady(true);
    }).catch((e) => setMsg({ ok: false, text: e.message }));
    const ms = models.current;
    return () => { dead = true; editor.current?.dispose(); editor.current = null; ms.forEach((m) => m.dispose()); ms.clear(); };
  }, []);

  async function openFile(path: string) {
    if (!monaco.current) return;
    setMsg(null);
    if (!models.current.has(path)) {
      try {
        const r = await api(`/projects/${project.id}/file?path=${encodeURIComponent(path)}`);
        const m = monaco.current.editor.createModel(r.content, undefined, monaco.current.Uri.file("/" + path));
        saved.current.set(path, r.content);
        m.onDidChangeContent(() => setDirty((d) => ({ ...d, [path]: m.getValue() !== saved.current.get(path) })));
        models.current.set(path, m);
      } catch (e: any) { setMsg({ ok: false, text: `${path}: ${e.message}` }); return; }
    }
    editor.current?.setModel(models.current.get(path));
    setTabs((t) => (t.includes(path) ? t : [...t, path]));
    setActive(path);
    editor.current?.focus();
  }

  function closeTab(path: string) {
    if (dirty[path] && !confirm(`Discard unsaved changes in ${path}?`)) return;
    models.current.get(path)?.dispose(); models.current.delete(path); saved.current.delete(path);
    setDirty((d) => { const n = { ...d }; delete n[path]; return n; });
    const rest = tabs.filter((t) => t !== path);
    setTabs(rest);
    if (active === path) {
      const next = rest[rest.length - 1] ?? "";
      setActive(next);
      editor.current?.setModel(next ? models.current.get(next) : null);
    }
  }

  saveRef.current = async () => {
    const path = active, m = models.current.get(path);
    if (!path || !m) return;
    try {
      const content = m.getValue();
      await api(`/projects/${project.id}/file`, { method: "PUT", body: { path, content } });
      saved.current.set(path, content);
      setDirty((d) => ({ ...d, [path]: false }));
      setMsg({ ok: true, text: `Saved ${path}` });
    } catch (e: any) { setMsg({ ok: false, text: `Couldn't save ${path}: ${e.message}` }); }
  };

  function toggle(dir: string) {
    const o = !open[dir];
    setOpen((s) => ({ ...s, [dir]: o }));
    if (o && !tree[dir]) loadDir(dir);
  }

  function renderDir(dir: string, depth: number): React.ReactNode {
    return (tree[dir] ?? []).map((e) => {
      const p = join(dir, e.name);
      return (
        <div key={p}>
          <button className={`ftree-row ${p === active ? "on" : ""}`} style={{ paddingLeft: 8 + depth * 12 }}
            onClick={() => (e.dir ? toggle(p) : openFile(p))} title={p}>
            <span className="ftree-ico">{e.dir ? (open[p] ? "▾" : "▸") : ""}</span>{e.name}
          </button>
          {e.dir && open[p] && renderDir(p, depth + 1)}
        </div>
      );
    });
  }

  async function newFile() {
    const name = prompt("New file path (relative to the project root):");
    if (!name?.trim()) return;
    const path = name.trim().replace(/^\/+/, "");
    try {
      await api(`/projects/${project.id}/file`, { method: "PUT", body: { path, content: "" } });
      const dir = path.includes("/") ? path.slice(0, path.lastIndexOf("/")) : "";
      await loadDir(dir);
      await openFile(path);
    } catch (e: any) { setMsg({ ok: false, text: e.message }); }
  }

  const anyDirty = Object.values(dirty).some(Boolean);
  useEffect(() => {
    if (!anyDirty) return;
    const h = (e: BeforeUnloadEvent) => { e.preventDefault(); };
    window.addEventListener("beforeunload", h);
    return () => window.removeEventListener("beforeunload", h);
  }, [anyDirty]);

  return (
    <div className="editor-view">
      <aside className="ftree">
        <div className="row between ftree-head"><b>{project.name}</b>
          <span className="row"><button className="small-btn" onClick={newFile} title="New file">+</button>
            <button className="small-btn" onClick={() => { setTree({}); loadDir(""); Object.keys(open).forEach((d) => d && open[d] && loadDir(d)); }} title="Refresh">↻</button></span></div>
        {renderDir("", 0)}
      </aside>
      <div className="editor-main">
        <div className="etabs">
          {tabs.map((t) => (
            <div key={t} className={`etab ${t === active ? "on" : ""}`}>
              <button onClick={() => openFile(t)} title={t}>{base(t)}{dirty[t] ? " ●" : ""}</button>
              <button className="etab-x" onClick={() => closeTab(t)} aria-label={`Close ${t}`}>×</button>
            </div>
          ))}
          <span style={{ flex: 1 }} />
          {msg && <span className={`${msg.ok ? "ok" : "error"} small`} style={{ margin: 0 }}>{msg.text}</span>}
          <button className="primary small-btn" disabled={!active || !dirty[active]} onClick={() => saveRef.current()} title="Save (Ctrl/Cmd+S)">Save</button>
        </div>
        <div className="editor-host">
          <div ref={host} style={{ position: "absolute", inset: 0, visibility: active ? "visible" : "hidden" }} />
          {(!active || !ready) && <div className="muted editor-empty">{ready ? "Pick a file in the tree." : "Loading editor…"}</div>}
        </div>
        <div className="editor-term">
          <div className="row between"><span className="muted small">Terminal — {project.name}</span>
            <button className="small-btn" onClick={async () => { await api(`/terminal?project=${project.id}&mode=shell`, { method: "DELETE" }).catch(() => {}); setShellEnded(false); setShellN((n) => n + 1); }}>Restart shell</button></div>
          <TermView key={`sh-${project.id}-${shellN}`} target={String(project.id)} shell onExit={() => setShellEnded(true)} />
          {shellEnded && <div className="muted small">Shell ended — use Restart shell.</div>}
        </div>
      </div>
    </div>
  );
}
