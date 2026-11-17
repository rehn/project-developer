"use client";
import { useEffect, useRef } from "react";
import "@xterm/xterm/css/xterm.css";

function wsBase() {
  if (process.env.NEXT_PUBLIC_BACKEND_WS) return process.env.NEXT_PUBLIC_BACKEND_WS;
  const proto = location.protocol === "https:" ? "wss" : "ws";
  // Dev: Next can't proxy WebSockets, so go straight to the backend. Production: same origin.
  return process.env.NODE_ENV === "development" ? `${proto}://${location.hostname}:3001` : `${proto}://${location.host}`;
}

/** xterm.js attached to a server-side PTY running Claude Code. */
export default function TermView({ target, cont, onExit }: { target: string; cont?: boolean; onExit?: (code: number) => void }) {
  const host = useRef<HTMLDivElement>(null);

  useEffect(() => {
    let disposed = false;
    let cleanup = () => {};
    (async () => {
      const [{ Terminal }, { FitAddon }, { WebLinksAddon }] = await Promise.all([
        import("@xterm/xterm"), import("@xterm/addon-fit"), import("@xterm/addon-web-links"),
      ]);
      if (disposed || !host.current) return;
      const term = new Terminal({
        cursorBlink: true, fontSize: 13, scrollback: 5000,
        fontFamily: "ui-monospace, SFMono-Regular, Menlo, monospace",
        theme: { background: "#010409", foreground: "#e6edf3" },
      });
      const fit = new FitAddon();
      term.loadAddon(fit);
      term.loadAddon(new WebLinksAddon());
      term.open(host.current);
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
      ws.onclose = () => { if (!ended && !disposed) term.write("\r\n\x1b[90m[disconnected — reload to reattach]\x1b[0m\r\n"); };
      term.onData((d) => ws.readyState === WebSocket.OPEN && ws.send(enc.encode(d)));
      const sendSize = () => ws.readyState === WebSocket.OPEN && ws.send(JSON.stringify({ type: "resize", cols: term.cols, rows: term.rows }));
      term.onResize(sendSize);
      const ro = new ResizeObserver(() => { try { fit.fit(); } catch {} });
      ro.observe(host.current);
      term.focus();

      cleanup = () => { ro.disconnect(); ws.close(); term.dispose(); };
    })();
    return () => { disposed = true; cleanup(); };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [target, cont]);

  return <div ref={host} className="xterm-host" />;
}
