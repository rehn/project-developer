"use client";
import { useEffect, useState } from "react";
import { useRouter } from "next/navigation";
import { api } from "@/lib/api";
import HeaderBar from "@/components/HeaderBar";
import ProjectTabs from "@/components/ProjectTabs";
import { ProjectProvider } from "@/lib/ProjectContext";

function Shell({ user, children }: { user: string; children: React.ReactNode }) {
  return (
    <div className="app">
      <HeaderBar user={user} />
      <ProjectTabs />
      <main>{children}</main>
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
