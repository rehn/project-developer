"use client";
import { useEffect, useState } from "react";
import Link from "next/link";
import { usePathname, useRouter } from "next/navigation";
import { api } from "@/lib/api";
import HeaderBar from "@/components/HeaderBar";
import { ProjectProvider, useProjects } from "@/lib/ProjectContext";

const NAV = [
  ["/terminal", "Terminal"], ["/projects", "Projects"], ["/pulls", "Pull requests"],
  ["/usage", "Usage"], ["/settings", "Settings"],
] as const;

function Shell({ user, children }: { user: string; children: React.ReactNode }) {
  const path = usePathname();
  const { projects, selected, select } = useProjects();
  return (
    <div className="app">
      <HeaderBar user={user} />
      <div className="shell">
        <aside>
          <h1>Project Developer</h1>
          <select value={selected?.id ?? ""} onChange={(e) => select(Number(e.target.value))} disabled={!projects.length}>
            {!projects.length && <option>No projects yet</option>}
            {projects.map((p) => <option key={p.id} value={p.id}>{p.name}</option>)}
          </select>
          <nav>
            {NAV.map(([href, label]) => (
              <Link key={href} href={href} className={path.startsWith(href) ? "active" : ""}>{label}</Link>
            ))}
          </nav>
        </aside>
        <main>{children}</main>
      </div>
    </div>
  );
}

export default function AppLayout({ children }: { children: React.ReactNode }) {
  const router = useRouter();
  const [user, setUser] = useState<string | null>(null);
  useEffect(() => {
    api("/auth/me").then((d) => (d.user ? setUser(d.user.username) : router.replace("/login"))).catch(() => router.replace("/login"));
  }, [router]);
  if (!user) return null;
  return <ProjectProvider><Shell user={user}>{children}</Shell></ProjectProvider>;
}
