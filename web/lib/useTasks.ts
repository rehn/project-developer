"use client";
import { useCallback, useEffect, useState } from "react";
import { api } from "@/lib/api";

export type TaskRequest = { id: number; from: string; toProject: number; to: string; body: string; createdAt: number };

/** Pending task requests that agents have queued for other projects; polled so new ones show up. */
export function useTasks() {
  const [tasks, setTasks] = useState<TaskRequest[]>([]);
  const reload = useCallback(() => api("/tasks").then(setTasks).catch(() => {}), []);
  useEffect(() => {
    reload();
    const t = setInterval(() => { if (!document.hidden) reload(); }, 10_000);
    return () => clearInterval(t);
  }, [reload]);
  return { tasks, reload };
}
