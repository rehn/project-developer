"use client";
import { useEffect, useState } from "react";
import { useRouter } from "next/navigation";
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
      <h1 className="brand"><Logo size={28} /> Project Developer</h1>
      <div className="gauges">
        {!l && <span className="muted small">Loading usage…</span>}
        {l?.error && <span className="muted small" title={l.error}>Plan usage unavailable — {l.error}</span>}
        {l?.fiveHour && <Gauge label="Session (5h)" pct={l.fiveHour.utilization} detail={`resets in ${resetIn(l.fiveHour.resetsAt)}`} />}
        {l?.sevenDay && <Gauge label="Weekly" pct={l.sevenDay.utilization} detail={`resets in ${resetIn(l.sevenDay.resetsAt)}`} />}
        {x && <Gauge label="Extra usage" pct={x.utilization} detail={`${money(x.used)} / ${money(x.monthlyLimit)}`} />}
      </div>
      <div className="row">
        <span className="muted">{user}</span>
        <button onClick={async () => { await api("/auth/logout", { method: "POST", body: {} }); router.replace("/login"); }}>Log out</button>
      </div>
    </header>
  );
}
