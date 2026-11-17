"use client";
import { useEffect, useState } from "react";
import Link from "next/link";
import { usePathname, useRouter } from "next/navigation";
import { api } from "@/lib/api";
import HeaderBar from "@/components/HeaderBar";
import ProjectTabs from "@/components/ProjectTabs";
import { ProjectProvider } from "@/lib/ProjectContext";

const NAV = [
  ["/terminal", "Terminal"], ["/projects", "Projects"], ["/pulls", "Pull requests"],
  ["/usage", "Usage"], ["/settings", "Settings"],
] as const;

function Shell({ user, children }: { user: string; children: React.ReactNode }) {
  const path = usePathname();
  return (
    <div className="app">
      <HeaderBar user={user} />
      <ProjectTabs />
      <div className="shell">
        <aside>
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
