"use client"

import * as React from "react"
import { Eye, EyeOff } from "lucide-react"
import { useRouter } from "next/navigation"

import { Button } from "@/components/ui/button"
import { capcomApi } from "@/lib/api-client"

type Mode = "login" | "signup"

export function AuthScreen() {
  const router = useRouter()
  const [mode, setMode] = React.useState<Mode>("login")
  const [email, setEmail] = React.useState("")
  const [password, setPassword] = React.useState("")
  const [confirmation, setConfirmation] = React.useState("")
  const [terms, setTerms] = React.useState(false)
  const [visible, setVisible] = React.useState(false)
  const [submitting, setSubmitting] = React.useState(false)
  const [error, setError] = React.useState("")
  const passwordStrength = React.useMemo(() => {
    if (!password) return 0
    let score = password.length >= 15 ? 1 : 0
    if (password.length >= 22) score++
    if (new Set(password).size >= 10) score++
    if (/\s/.test(password) || /[^\p{L}\p{N}\s]/u.test(password)) score++
    return Math.min(score, 4)
  }, [password])

  async function submit(event: React.FormEvent) {
    event.preventDefault()
    setError("")
    if (mode === "signup" && password !== confirmation) {
      setError("The passwords do not match.")
      return
    }
    if (mode === "signup" && !terms) {
      setError("Accept the terms to create your workspace.")
      return
    }
    setSubmitting(true)
    try {
      if (mode === "signup") await capcomApi.signup(email, password)
      else await capcomApi.login(email, password)
      router.push("/")
      router.refresh()
    } catch (failure) {
      setError(failure instanceof Error ? failure.message : "Unable to continue. Try again.")
    } finally {
      setSubmitting(false)
    }
  }

  function switchMode(next: Mode) {
    setMode(next)
    setError("")
    setConfirmation("")
  }

  return (
    <main className="auth-layout">
      <section className="auth-visual" aria-label="Live agent control plane visualization">
        <div className="auth-brand"><span className="auth-brand-dot" /> CAPCOM <span>CONTROL PLANE</span></div>
        <div className="topology-stage" aria-hidden="true">
          <svg className="topology-lines" viewBox="0 0 720 520" preserveAspectRatio="none">
            <path d="M360 80 V180 M360 180 L155 300 M360 180 L360 300 M360 180 L565 300" />
            <path className="topology-signal signal-a" d="M360 80 V180 L155 300" />
            <path className="topology-signal signal-b" d="M360 80 V180 L360 300" />
            <path className="topology-signal signal-c" d="M360 80 V180 L565 300" />
          </svg>
          <Node className="runtime-node" eyebrow="RUNTIME" title="LANGGRAPH" status="CONNECTED" />
          <Node className="agent-node agent-one" eyebrow="MAIN AGENT" title="RESEARCH" status="RUNNING" />
          <Node className="agent-node agent-two" eyebrow="DELEGATE" title="ANALYST" status="SYNCED" />
          <Node className="agent-node agent-three" eyebrow="DELEGATE" title="WRITER" status="RUNNING" />
          <div className="telemetry-strip"><span>LIVE TELEMETRY</span><i /><i /><i /><strong>99.98%</strong></div>
        </div>
        <div className="auth-visual-copy">
          <p className="font-hud">ONE VIEW · EVERY RUNTIME</p>
          <h1>Command the agent fleet<br />with confidence.</h1>
          <p>Observe topology, inspect usage, and control runtime access from one calm operational surface.</p>
        </div>
      </section>

      <section className="auth-panel">
        <div className="auth-card">
          <div className="auth-mobile-brand"><span className="auth-brand-dot" /> CAPCOM</div>
          <div className="auth-mode" role="tablist" aria-label="Authentication mode">
            <button role="tab" aria-selected={mode === "login"} onClick={() => switchMode("login")}>Log in</button>
            <button role="tab" aria-selected={mode === "signup"} onClick={() => switchMode("signup")}>Create account</button>
          </div>
          <header>
            <p className="font-hud">SECURE OPERATOR ACCESS</p>
            <h2>{mode === "login" ? "Welcome back" : "Create your control plane"}</h2>
            <span>{mode === "login" ? "Sign in to your Capcom workspace." : "Your personal workspace is created automatically."}</span>
          </header>
          <form onSubmit={submit} className="auth-form">
            <label>Email address<input type="email" autoComplete="email" value={email} onChange={(e) => setEmail(e.target.value)} required autoFocus /></label>
            <label>Password
              <span className="password-field">
                <input type={visible ? "text" : "password"} autoComplete={mode === "login" ? "current-password" : "new-password"} minLength={15} maxLength={128} value={password} onChange={(e) => setPassword(e.target.value)} required />
                <button type="button" aria-label={visible ? "Hide password" : "Show password"} onClick={() => setVisible(!visible)}>{visible ? <EyeOff /> : <Eye />}</button>
              </span>
              {mode === "signup" && <>
                <span className="password-strength" aria-label={`Password strength ${passwordStrength} of 4`}>
                  {[1, 2, 3, 4].map((level) => <i key={level} data-active={level <= passwordStrength} />)}
                </span>
                <small>15–128 characters. Passphrases work well.</small>
              </>}
            </label>
            {mode === "signup" && <label>Confirm password<input type={visible ? "text" : "password"} autoComplete="new-password" value={confirmation} onChange={(e) => setConfirmation(e.target.value)} required /></label>}
            {mode === "signup" && <label className="terms-row"><input type="checkbox" checked={terms} onChange={(e) => setTerms(e.target.checked)} /> <span>I agree to the Terms and Privacy Policy.</span></label>}
            {error && <div className="auth-error" role="alert">{error}</div>}
            <Button className="auth-submit" type="submit" disabled={submitting}>{submitting ? "Securing session…" : mode === "login" ? "Enter control plane" : "Create workspace"}</Button>
          </form>
        </div>
      </section>
    </main>
  )
}

function Node({ className, eyebrow, title, status }: { className: string; eyebrow: string; title: string; status: string }) {
  return <div className={className}><span>{eyebrow}</span><strong>{title}</strong><em><i /> {status}</em></div>
}
