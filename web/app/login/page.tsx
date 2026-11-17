"use client";
import { useEffect, useState } from "react";
import { useRouter } from "next/navigation";
import { api } from "@/lib/api";
import Logo from "@/components/Logo";

export default function Login() {
  const router = useRouter();
  const [setup, setSetup] = useState(false);
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState("");

  useEffect(() => {
    api("/auth/me").then((d) => (d.user ? router.replace("/terminal") : setSetup(d.needsSetup))).catch(() => {});
  }, [router]);

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    setError("");
    try {
      await api(setup ? "/auth/setup" : "/auth/login", { body: { username, password } });
      router.replace("/terminal");
    } catch (err: any) { setError(err.message); }
  }

  return (
    <form className="card login" onSubmit={submit}>
      <h1 className="brand"><Logo size={28} /> Project Developer</h1>
      <p className="muted">{setup ? "First run — create your account." : "Sign in to continue."}</p>
      <input placeholder="Username" autoComplete="username" value={username} onChange={(e) => setUsername(e.target.value)} autoFocus />
      <input type="password" placeholder={setup ? "Password (min 8 chars)" : "Password"} autoComplete={setup ? "new-password" : "current-password"}
        value={password} onChange={(e) => setPassword(e.target.value)} />
      {error && <div className="error">{error}</div>}
      <button className="primary">{setup ? "Create account" : "Sign in"}</button>
    </form>
  );
}
