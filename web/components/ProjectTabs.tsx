"use client";
import { useEffect, useRef, useState } from "react";
import Link from "next/link";
import { useProjects } from "@/lib/ProjectContext";

/** One tab per project under the header. Tabs can be hidden (×) and brought back from the "hidden" menu. */
export default function ProjectTabs() {
  const { projects, selected, select, hidden, hide, unhide } = useProjects();
  const [open, setOpen] = useState(false);
  const menu = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (!open) return;
    const close = (e: MouseEvent) => { if (!menu.current?.contains(e.target as Node)) setOpen(false); };
    const esc = (e: KeyboardEvent) => { if (e.key === "Escape") setOpen(false); };
    document.addEventListener("mousedown", close); document.addEventListener("keydown", esc);
    return () => { document.removeEventListener("mousedown", close); document.removeEventListener("keydown", esc); };
  }, [open]);

  const visible = projects.filter((p) => !hidden.includes(p.id));
  const hiddenList = projects.filter((p) => hidden.includes(p.id));

  return (
    <div className="ptabs">
      <div className="ptabs-scroll" role="tablist">
        {!projects.length && <span className="muted small">No projects yet — <Link href="/projects">add one</Link></span>}
        {!!projects.length && !visible.length && <span className="muted small">All projects are hidden</span>}
        {visible.map((p) => {
          const active = p.id === selected?.id;
          return (
            <div key={p.id} className={`ptab ${active ? "active" : ""}`}>
              <button role="tab" aria-selected={active} className="ptab-name" onClick={() => select(p.id)} title={p.repoUrl}>{p.name}</button>
              <button className="ptab-x" onClick={() => hide(p.id)} title={`Hide ${p.name}`} aria-label={`Hide ${p.name}`}>×</button>
            </div>
          );
        })}
      </div>
      {hiddenList.length > 0 && (
        <div className="ptabs-menu" ref={menu}>
          <button onClick={() => setOpen((o) => !o)} aria-expanded={open}>{hiddenList.length} hidden ▾</button>
          {open && (
            <div className="ptabs-pop">
              {hiddenList.map((p) => (
                <button key={p.id} onClick={() => { select(p.id); setOpen(false); }}>{p.name}</button>
              ))}
              {hiddenList.length > 1 && (
                <button className="muted" onClick={() => { hiddenList.forEach((p) => unhide(p.id)); setOpen(false); }}>Show all</button>
              )}
            </div>
          )}
        </div>
      )}
    </div>
  );
}
