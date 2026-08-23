import { useEffect, useRef, useState } from "react";
import type { LogEvent } from "../api";

const SOURCE_TONE: Record<string, string> = {
  local: "text-accent",
  forward: "text-sky-400",
  cache: "text-amber-400",
  error: "text-red-400",
};

export function LogsPanel() {
  const [events, setEvents] = useState<LogEvent[]>([]);
  const [connected, setConnected] = useState(false);
  const boxRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    let cancelled = false;
    let es: EventSource | null = null;

    fetch("/api/v1/logs?limit=40")
      .then((r) => (r.ok ? r.json() : { events: [] }))
      .then((d) => { if (!cancelled) setEvents(d.events ?? []); })
      .catch(() => {});

    const connect = () => {
      es = new EventSource("/api/v1/logs/stream");
      es.onopen = () => setConnected(true);
      es.onmessage = (m) => {
        try {
          const ev: LogEvent = JSON.parse(m.data);
          setEvents((prev) => [...prev.slice(-199), ev]);
        } catch { /* ignore */ }
      };
      es.onerror = () => {
        setConnected(false);
        es?.close();
        setTimeout(connect, 2000);
      };
    };
    connect();

    return () => {
      cancelled = true;
      es?.close();
    };
  }, []);

  useEffect(() => {
    boxRef.current?.scrollTo({ top: boxRef.current.scrollHeight });
  }, [events]);

  return (
    <section className="flex min-h-0 flex-col rounded-2xl border border-edge bg-surface">
      <header className="flex items-center justify-between px-5 py-4">
        <h2 className="text-sm font-semibold tracking-wide">Query log</h2>
        <span className={`flex items-center gap-1.5 text-[11px] ${connected ? "text-accent" : "text-muted"}`}>
          <span className={`h-1.5 w-1.5 rounded-full ${connected ? "bg-accent animate-pulse" : "bg-edge"}`} />
          {connected ? "live" : "connecting…"}
        </span>
      </header>
      <div ref={boxRef} className="min-h-0 flex-1 overflow-y-auto px-5 pb-4 font-mono text-[11px] leading-relaxed">
        {events.length === 0 && <p className="py-6 text-center text-muted">Waiting for queries…</p>}
        {events.map((ev, i) => (
          <div key={i} className="flex gap-3 whitespace-nowrap py-0.5">
            <span className="shrink-0 text-muted/70">{new Date(ev.time).toLocaleTimeString()}</span>
            <span className={`w-14 shrink-0 ${SOURCE_TONE[ev.source] ?? "text-muted"}`}>{ev.source}</span>
            <span className="shrink-0 truncate text-ink">{ev.name}</span>
            <span className="w-10 shrink-0 text-muted">{ev.type}</span>
            <span className="truncate text-muted">{ev.answer}</span>
            <span className="ml-auto shrink-0 text-muted/70">{(ev.latency_ns / 1e6).toFixed(1)}ms</span>
          </div>
        ))}
      </div>
    </section>
  );
}
