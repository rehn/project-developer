"use client";
import { createContext, useCallback, useContext, useEffect, useState, type ReactNode } from "react";
import { api, type Project } from "./api";

type Ctx = {
  projects: Project[]; selected: Project | null; select: (id: number) => void; reload: () => Promise<void>;
  /** Projects hidden from the tab bar (per browser). Hiding never deletes anything. */
  hidden: number[]; hide: (id: number) => void; unhide: (id: number) => void;
};
const C = createContext<Ctx>(null as any);
export const useProjects = () => useContext(C);

const save = (k: string, v: string) => { try { localStorage.setItem(k, v); } catch {} };

export function ProjectProvider({ children }: { children: ReactNode }) {
  const [projects, setProjects] = useState<Project[]>([]);
  const [selId, setSelId] = useState<number | null>(null);
  const [hidden, setHidden] = useState<number[]>([]);

  const reload = useCallback(async () => {
    const p: Project[] = await api("/projects");
    setProjects(p);
  }, []);
  useEffect(() => {
    reload().catch(() => {});
    try {
      const v = localStorage.getItem("pd_selected"); if (v) setSelId(Number(v));
      const h = JSON.parse(localStorage.getItem("pd_hidden") || "[]");
      if (Array.isArray(h)) setHidden(h.filter((x) => typeof x === "number"));
    } catch {}
  }, [reload]);

  const visible = projects.filter((p) => !hidden.includes(p.id));
  const selected = projects.find((p) => p.id === selId) ?? visible[0] ?? projects[0] ?? null;

  const unhide = (id: number) =>
    setHidden((h) => { if (!h.includes(id)) return h; const n = h.filter((x) => x !== id); save("pd_hidden", JSON.stringify(n)); return n; });
  // Picking a hidden project brings its tab back.
  const select = (id: number) => { setSelId(id); save("pd_selected", String(id)); unhide(id); };
  const hide = (id: number) => {
    const n = hidden.includes(id) ? hidden : [...hidden, id];
    setHidden(n); save("pd_hidden", JSON.stringify(n));
    // Hiding the open project moves to the next visible one.
    if (selected?.id === id) {
      const next = projects.find((p) => !n.includes(p.id));
      if (next) { setSelId(next.id); save("pd_selected", String(next.id)); }
    }
  };
  return <C.Provider value={{ projects, selected, select, reload, hidden, hide, unhide }}>{children}</C.Provider>;
}
