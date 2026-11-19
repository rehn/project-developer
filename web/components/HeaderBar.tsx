"use client";
import { useEffect, useState } from "react";
import Link from "next/link";
import { usePathname, useRouter } from "next/navigation";
import { api } from "@/lib/api";
import Logo from "@/components/Logo";

function resetIn(iso: string) {
  const ms = Date.parse(iso) - Date.now();
  if (!(ms > 0)) return "resetting…";
  const m = Math.round(ms / 60000), d = Math.floor(m / 1440), h = Math.floor((m % 1440) / 60);
  return d ? `${d}d ${h}h` : h ? `${h}h ${m % 60}m` : `${m}m`;
}
const level = (p: number) => (p >= 90 ? "hot" : p >= 70 ? "warn" : "");

function Gauge({ label, pct, detail }: { label: string; pct: number; detail: string }) {
  const c = Math.max(0, Math.min(100, pct));
  return (
    <div className="gauge" title={`${label}: ${Math.round(pct)}% used — ${detail}`}>
      <div className="row between"><span>{label}</span><b>{Math.round(pct)}%</b></div>
      <div className="track"><div className={`fill ${level(c)}`} style={{ width: `${c}%` }} /></div>
      <div className="muted small">{detail}</div>
    </div>
  );
}

export default function HeaderBar({ user }: { user: string }) {
  const router = useRouter();
  const path = usePathname();
  const [l, setL] = useState<any>(null);
  useEffect(() => {
    const load = () => api("/usage/limits").then(setL).catch((e) => setL({ error: e.message }));
    load();
    const t = setInterval(load, 60_000);
    window.addEventListener("pd:auth-changed", load);
    return () => { clearInterval(t); window.removeEventListener("pd:auth-changed", load); };
  }, []);

  const x = l?.extraUsage;
  const money = (n: number) =>
    (n / 10 ** x.decimalPlaces).toLocaleString(undefined, { style: "currency", currency: x.currency, maximumFractionDigits: 0 });

  return (
    <header className="topbar">
      <h1 className="brand"><Link href="/terminal"><Logo size={28} /> Project Developer</Link></h1>
      <div className="gauges">
        {!l && <span className="muted small">Loading usage…</span>}
        {l?.stale && <span className="muted small" title={`${l.stale} — showing the last known values`}>⚠ stale</span>}
        {l?.error &&<span className="muted small" title={l.error}>Plan usage unavailable — {l.error}</span>}
        {l?.fiveHour && <Gauge label="Session (5h)" pct={l.fiveHour.utilization} detail={`resets in ${resetIn(l.fiveHour.resetsAt)}`} />}
        {l?.sevenDay && <Gauge label="Weekly" pct={l.sevenDay.utilization} detail={`resets in ${resetIn(l.sevenDay.resetsAt)}`} />}
        {x && <Gauge label="Extra usage" pct={x.utilization} detail={`${money(x.used)} / ${money(x.monthlyLimit)}`} />}
      </div>
      <div className="row">
        <Link href="/settings" className={`hdr-link ${path.startsWith("/settings") ? "active" : ""}`} title="Settings">
          <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true"><circle cx="12" cy="12" r="3" /><path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 1 1-2.83 2.83l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 1 1-4 0v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 1 1-2.83-2.83l.06-.06A1.65 1.65 0 0 0 4.68 15a1.65 1.65 0 0 0-1.51-1H3a2 2 0 1 1 0-4h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 1 1 2.83-2.83l.06.06A1.65 1.65 0 0 0 9 4.68a1.65 1.65 0 0 0 1-1.51V3a2 2 0 1 1 4 0v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 1 1 2.83 2.83l-.06.06A1.65 1.65 0 0 0 19.4 9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 1 1 0 4h-.09a1.65 1.65 0 0 0-1.51 1z" /></svg>
          Settings
        </Link>
        <span className="muted">{user}</span>
        <button onClick={async () => { await api("/auth/logout", { method: "POST", body: {} }); router.replace("/login"); }}>Log out</button>
      </div>
    </header>
  );
}
