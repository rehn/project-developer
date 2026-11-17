"use client";
import { createContext, useCallback, useContext, useEffect, useState, type ReactNode } from "react";
import { api, type Project } from "./api";

type Ctx = { projects: Project[]; selected: Project | null; select: (id: number) => void; reload: () => Promise<void> };
const C = createContext<Ctx>(null as any);
export const useProjects = () => useContext(C);

export function ProjectProvider({ children }: { children: ReactNode }) {
  const [projects, setProjects] = useState<Project[]>([]);
  const [selId, setSelId] = useState<number | null>(null);

  const reload = useCallback(async () => {
    const p: Project[] = await api("/projects");
    setProjects(p);
  }, []);
  useEffect(() => {
    reload().catch(() => {});
    try { const v = localStorage.getItem("pd_selected"); if (v) setSelId(Number(v)); } catch {}
  }, [reload]);

  const selected = projects.find((p) => p.id === selId) ?? projects[0] ?? null;
  const select = (id: number) => { setSelId(id); try { localStorage.setItem("pd_selected", String(id)); } catch {} };
  return <C.Provider value={{ projects, selected, select, reload }}>{children}</C.Provider>;
}
