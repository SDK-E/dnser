import { useCallback, useEffect, useState } from "react";
import { desktop, type DesktopStatus, type SetupStep } from "../api";
import { Badge, Button, Toggle } from "./ui";

export function DesktopPanel({ onChanged }: { onChanged?: () => void }) {
  const [ds, setDs] = useState<DesktopStatus | null>(null);
  const [gone, setGone] = useState(false);
  const [busy, setBusy] = useState<"" | "setup" | "revert">("");
  const [steps, setSteps] = useState<SetupStep[] | null>(null);
  const [err, setErr] = useState<string | null>(null);

  const refresh = useCallback(async () => {
    try {
      const s = await desktop.status();
      if (s === null) {
        setGone(true);
        return;
      }
      setDs(s);
    } catch (e) {
      setErr(e instanceof Error ? e.message : String(e));
    }
  }, []);

  useEffect(() => {
    refresh();
  }, [refresh]);

  if (gone) return null;

  const runSetup = async () => {
    setBusy("setup");
    setErr(null);
    try {
      const res = await desktop.runSetup();
      setSteps(res.steps);
      await refresh();
      onChanged?.();
    } catch (e) {
      setErr(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy("");
    }
  };

  const revert = async () => {
    setBusy("revert");
    setErr(null);
    try {
      await desktop.revert();
      setSteps(null);
      await refresh();
      onChanged?.();
    } catch (e) {
      setErr(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy("");
    }
  };

  const toggleAutostart = async (v: boolean) => {
    if (!ds) return;
    try {
      await desktop.setAutostart(v);
      await refresh();
    } catch (e) {
      setErr(e instanceof Error ? e.message : String(e));
    }
  };

  const s = ds?.setup;
  const allGood = !!s && s.ca_trusted && s.routed;

  return (
    <div className="mb-6 rounded-xl border border-edge bg-surface px-5 py-4">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div className="flex flex-wrap items-center gap-2">
          <span className="text-sm font-semibold">
            DNS<span className="text-accent">.</span>er desktop
          </span>
          {s?.ca_trusted ? (
            <Badge tone="green">CA trusted{s?.ca_trust_mode ? ` · ${s.ca_trust_mode}` : ""}</Badge>
          ) : (
            <Badge tone="amber">CA not trusted</Badge>
          )}
          {s?.routed ? (
            <Badge tone="green">{routingLabel(s.routing_mode)}</Badge>
          ) : (
            <Badge tone="amber">DNS not routed</Badge>
          )}
          {ds && <Toggle checked={ds.autostart} onChange={toggleAutostart} label="launch at login" />}
        </div>
        <div className="flex items-center gap-2">
          {!allGood && (
            <span className="text-xs text-muted">
              one admin prompt configures everything{ds?.setup.needs_port_53 ? " · needs port 53" : ""}
            </span>
          )}
          <Button variant="primary" onClick={runSetup} disabled={busy !== ""}>
            {busy === "setup" ? "setting up…" : allGood ? "re-run setup" : "set up this machine"}
          </Button>
          {(allGood || steps) && (
            <Button variant="danger" onClick={revert} disabled={busy !== ""}>
              {busy === "revert" ? "restoring…" : "restore system"}
            </Button>
          )}
        </div>
      </div>

      {err && <p className="mt-3 text-xs text-red-400">{err}</p>}

      {steps && (
        <ul className="mt-3 space-y-1 border-t border-edge pt-3 font-mono text-xs">
          {steps.map((st, i) => (
            <li key={i} className={st.err ? "text-red-400" : "text-muted"}>
              {st.err ? "✗" : "✓"} <span className="text-ink">{st.name}</span>
              {(st.detail || st.err) && <> — {st.err || st.detail}</>}
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}

function routingLabel(mode: string): string {
  switch (mode) {
    case "resolver-files":
      return "*.test → local";
    case "system-resolver":
      return "system resolver → local";
    default:
      return mode;
  }
}
