"use client";
import { useEffect, useRef, useState } from "react";
import Link from "next/link";
import { usePathname, useRouter } from "next/navigation";
import { useProjects } from "@/lib/ProjectContext";

/** One tab per project under the header; tabs wrap onto more rows so every project stays visible.
 *  Tabs can be hidden (×) and brought back from the "hidden" menu;
 *  "+" opens the page for adding and removing projects. */
export default function ProjectTabs() {
  const { projects, selected, select: pick, hidden, hide, unhide } = useProjects();
  const router = useRouter();
  const path = usePathname();
  // A project lives in the terminal view, so picking one from another page goes there.
  const select = (id: number) => { pick(id); if (!path.startsWith("/terminal")) router.push("/terminal"); };
  const onTerminal = path.startsWith("/terminal");
  const [open, setOpen] = useState(false);
  const menu = useRef<HTMLDivElement>(null);
  const bar = useRef<HTMLDivElement>(null);

  // Tabs wrap onto extra rows when they don't fit; publish the bar's real height so the views below size to it.
  useEffect(() => {
    const el = bar.current; if (!el) return;
    const root = document.documentElement.style;
    const ro = new ResizeObserver(() => root.setProperty("--tabs-h", `${el.offsetHeight}px`));
    ro.observe(el);
    return () => { ro.disconnect(); root.removeProperty("--tabs-h"); };
  }, []);

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
    <div className="ptabs" ref={bar}>
      <div className="ptabs-scroll" role="tablist">
        <div className={`ptab ${path.startsWith("/general") ? "active" : ""}`}>
          <Link role="tab" aria-selected={path.startsWith("/general")} className="ptab-name ptab-general" href="/general" title="Claude session not tied to any project">General</Link>
        </div>
        {!projects.length && <span className="muted small">No projects yet — <Link href="/projects">add one</Link></span>}
        {!!projects.length && !visible.length && <span className="muted small">All projects are hidden</span>}
        {visible.map((p) => {
          const active = onTerminal && p.id === selected?.id;
          return (
            <div key={p.id} className={`ptab ${active ? "active" : ""}`}>
              <button role="tab" aria-selected={active} className="ptab-name" onClick={() => select(p.id)} title={p.repoUrl}>{p.name}</button>
              <button className="ptab-x" onClick={() => hide(p.id)} title={`Hide ${p.name}`} aria-label={`Hide ${p.name}`}>×</button>
            </div>
          );
        })}
        <Link href="/projects" className={`ptab-add ${path.startsWith("/projects") ? "active" : ""}`} title="Add or remove projects" aria-label="Add or remove projects">+</Link>
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
