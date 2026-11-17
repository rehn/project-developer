"use client";
import { useEffect, useRef, useState } from "react";
import "@xterm/xterm/css/xterm.css";

function wsBase() {
  if (process.env.NEXT_PUBLIC_BACKEND_WS) return process.env.NEXT_PUBLIC_BACKEND_WS;
  const proto = location.protocol === "https:" ? "wss" : "ws";
  // Dev: Next can't proxy WebSockets, so go straight to the backend. Production: same origin.
  return process.env.NODE_ENV === "development" ? `${proto}://${location.hostname}:3001` : `${proto}://${location.host}`;
}

const isImage = (f: File) => /^image\/(png|jpeg|gif|webp)$/.test(f.type);

async function uploadImage(f: File): Promise<string> {
  const fd = new FormData();
  fd.append("file", f, f.name || "pasted.png");
  const res = await fetch("/api/uploads", { method: "POST", body: fd });
  const data = await res.json().catch(() => ({}));
  if (!res.ok) throw new Error(data.error ?? `Upload failed (${res.status})`);
  return data.path;
}

/** xterm.js attached to a server-side PTY running Claude Code. Images can be pasted, dropped or attached:
 *  they are uploaded to the server and their path is typed into the prompt. */
export default function TermView({ target, cont, onExit }: { target: string; cont?: boolean; onExit?: (code: number) => void }) {
  const host = useRef<HTMLDivElement>(null);
  const picker = useRef<HTMLInputElement>(null);
  // Types text into the PTY as a paste; set once the socket is up.
  const typeIn = useRef<((s: string) => void) | null>(null);
  const [drag, setDrag] = useState(false);
  const [note, setNote] = useState<{ err: boolean; text: string } | null>(null);

  async function attach(files: File[]) {
    const imgs = files.filter(isImage);
    if (!imgs.length) { if (files.length) setNote({ err: true, text: "Only PNG, JPEG, GIF and WebP images can be attached" }); return; }
    if (!typeIn.current) { setNote({ err: true, text: "Terminal isn't connected" }); return; }
    setNote({ err: false, text: `Uploading ${imgs.length > 1 ? `${imgs.length} images` : "image"}…` });
    try {
      const paths = [];
      for (const f of imgs) paths.push(await uploadImage(f));
      typeIn.current(paths.join(" ") + " ");
      setNote(null);
    } catch (e: any) { setNote({ err: true, text: e.message }); }
  }
  const attachRef = useRef(attach);
  attachRef.current = attach;

  useEffect(() => {
    if (!note || note.err === false) return;
    const t = setTimeout(() => setNote(null), 6000);
    return () => clearTimeout(t);
  }, [note]);

  useEffect(() => {
    let disposed = false;
    let cleanup = () => {};
    (async () => {
      const [{ Terminal }, { FitAddon }, { WebLinksAddon }] = await Promise.all([
        import("@xterm/xterm"), import("@xterm/addon-fit"), import("@xterm/addon-web-links"),
      ]);
      if (disposed || !host.current) return;
      const el = host.current;
      const term = new Terminal({
        cursorBlink: true, fontSize: 13, scrollback: 5000,
        fontFamily: "ui-monospace, SFMono-Regular, Menlo, monospace",
        theme: { background: "#010409", foreground: "#e6edf3" },
      });
      const fit = new FitAddon();
      term.loadAddon(fit);
      term.loadAddon(new WebLinksAddon());
      term.open(el);
      fit.fit();

      const qs = new URLSearchParams({ project: target, cols: String(term.cols), rows: String(term.rows) });
      if (cont) qs.set("continue", "1");
      const ws = new WebSocket(`${wsBase()}/api/terminal?${qs}`);
      ws.binaryType = "arraybuffer";
      const enc = new TextEncoder();
      let ended = false;

      ws.onmessage = (e) => {
        if (typeof e.data === "string") {
          try {
            const m = JSON.parse(e.data);
            if (m.type === "exit") { ended = true; term.write(`\r\n\x1b[90m[session ended]\x1b[0m\r\n`); onExit?.(m.code); }
          } catch {}
        } else term.write(new Uint8Array(e.data));
      };
      ws.onclose = () => { typeIn.current = null; if (!ended && !disposed) term.write("\r\n\x1b[90m[disconnected — reload to reattach]\x1b[0m\r\n"); };
      const send = (d: string) => ws.readyState === WebSocket.OPEN && ws.send(enc.encode(d));
      term.onData(send);
      // Bracketed paste when the app asked for it, so Claude Code treats the path like a pasted file.
      typeIn.current = (s) => { send(term.modes.bracketedPasteMode ? `\x1b[200~${s}\x1b[201~` : s); term.focus(); };
      const sendSize = () => ws.readyState === WebSocket.OPEN && ws.send(JSON.stringify({ type: "resize", cols: term.cols, rows: term.rows }));
      term.onResize(sendSize);
      const ro = new ResizeObserver(() => { try { fit.fit(); } catch {} });
      ro.observe(el);

      // Capture phase: runs before xterm's own textarea handler, which would only paste text.
      const onPaste = (e: ClipboardEvent) => {
        const files = Array.from(e.clipboardData?.files ?? []).filter(isImage);
        if (!files.length) return;
        e.preventDefault(); e.stopPropagation();
        attachRef.current(files);
      };
      el.addEventListener("paste", onPaste, true);
      term.focus();

      cleanup = () => { el.removeEventListener("paste", onPaste, true); ro.disconnect(); ws.close(); term.dispose(); typeIn.current = null; };
    })();
    return () => { disposed = true; cleanup(); };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [target, cont]);

  const hasFiles = (e: React.DragEvent) => Array.from(e.dataTransfer.types).includes("Files");
  return (
    <div className={`xterm-host ${drag ? "dragging" : ""}`}
      onDragOver={(e) => { if (hasFiles(e)) { e.preventDefault(); setDrag(true); } }}
      onDragLeave={(e) => { if (!e.currentTarget.contains(e.relatedTarget as Node)) setDrag(false); }}
      onDrop={(e) => { if (!hasFiles(e)) return; e.preventDefault(); setDrag(false); attach(Array.from(e.dataTransfer.files)); }}>
      <div ref={host} className="xterm-inner" />
      <button className="term-attach" onClick={() => picker.current?.click()} title="Attach image (or paste / drop one into the terminal)" aria-label="Attach image">
        <svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
          <path d="M21.44 11.05l-9.19 9.19a6 6 0 0 1-8.49-8.49l9.19-9.19a4 4 0 0 1 5.66 5.66l-9.2 9.19a2 2 0 0 1-2.83-2.83l8.49-8.48" />
        </svg>
      </button>
      <input ref={picker} type="file" accept="image/png,image/jpeg,image/gif,image/webp" multiple hidden
        onChange={(e) => { attach(Array.from(e.target.files ?? [])); e.target.value = ""; }} />
      {drag && <div className="term-drop">Drop image to attach</div>}
      {note && <div className={`term-note ${note.err ? "err" : ""}`}>{note.text}</div>}
    </div>
  );
}
