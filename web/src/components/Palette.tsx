import { useEffect, useMemo, useRef, useState } from "react";
import type { AppInfo, Project } from "../api";
import { Kbd } from "./ui";

export interface Command {
  id: string;
  label: string;
  hint?: string;
  run: () => void;
}

export function Palette({
  open, onClose, projects, apps, tld,
  onOpenDoctor, onOpenLink, onOpenProject,
}: {
  open: boolean;
  onClose: () => void;
  projects: Project[];
  apps: Record<string, AppInfo>;
  tld: string;
  onOpenDoctor: () => void;
  onOpenLink: () => void;
  onOpenProject: (domain: string) => void;
}) {
  const [query, setQuery] = useState("");
  const [index, setIndex] = useState(0);
  const inputRef = useRef<HTMLInputElement>(null);

  const commands = useMemo<Command[]>(() => {
    const cmds: Command[] = [
      { id: "cmd:link", label: "Link a project…", hint: "detect stack & serve", run: onOpenLink },
      { id: "cmd:doctor", label: "Run doctor", hint: "system diagnostics", run: onOpenDoctor },
    ];
    for (const p of projects) {
      cmds.push({
        id: `open:${p.domain}`,
        label: `Open ${p.domain}`,
        hint: apps[p.domain] ? `${apps[p.domain].state}${apps[p.domain].port ? " :" + apps[p.domain].port : ""}` : `https://${p.domain}`,
        run: () => onOpenProject(p.domain),
      });
      if (apps[p.domain]?.command || p.run?.command) {
        cmds.push({
          id: `restart:${p.domain}`,
          label: `Restart ${p.domain}`,
          hint: "dev server",
          run: () => void import("../api").then(({ api }) => api.restartApp(p.domain).catch(() => undefined)),
        });
        cmds.push({
          id: `stop:${p.domain}`,
          label: `Stop ${p.domain}`,
          hint: "dev server",
          run: () => void import("../api").then(({ api }) => api.stopApp(p.domain).catch(() => undefined)),
        });
      }
    }
    return cmds;
  }, [projects, apps, tld, onOpenDoctor, onOpenLink, onOpenProject]);

  const filtered = useMemo(() => {
    const q = query.trim().toLowerCase();
    if (!q) return commands;
    return commands.filter((c) => c.label.toLowerCase().includes(q) || c.hint?.toLowerCase().includes(q));
  }, [commands, query]);

  useEffect(() => {
    if (open) {
      setQuery("");
      setIndex(0);
      requestAnimationFrame(() => inputRef.current?.focus());
    }
  }, [open]);

  useEffect(() => setIndex(0), [query]);

  if (!open) return null;

  const commit = (i: number) => {
    const cmd = filtered[i];
    if (!cmd) return;
    onClose();
    cmd.run();
  };

  return (
    <div className="fixed inset-0 z-60 flex items-start justify-center bg-black/60 pt-[15vh] animate-fade-in"
      onClick={onClose}>
      <div
        className="w-full max-w-lg overflow-hidden rounded-2xl border border-edge bg-surface shadow-2xl animate-pop-in"
        onClick={(e) => e.stopPropagation()}
      >
        <input
          ref={inputRef}
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === "ArrowDown") { e.preventDefault(); setIndex((i) => Math.min(i + 1, filtered.length - 1)); }
            else if (e.key === "ArrowUp") { e.preventDefault(); setIndex((i) => Math.max(i - 1, 0)); }
            else if (e.key === "Enter") { e.preventDefault(); commit(index); }
            else if (e.key === "Escape") { onClose(); }
          }}
          placeholder="Type a command…"
          className="w-full border-b border-edge bg-transparent px-4 py-3 text-sm outline-none placeholder:text-muted"
        />
        <ul className="max-h-72 overflow-y-auto py-1">
          {filtered.length === 0 && <li className="px-4 py-3 text-sm text-muted">no matching commands</li>}
          {filtered.map((c, i) => (
            <li key={c.id}>
              <button
                className={`flex w-full items-center justify-between px-4 py-2 text-left text-sm ${
                  i === index ? "bg-accent/10 text-accent" : "hover:bg-white/5"
                }`}
                onMouseEnter={() => setIndex(i)}
                onClick={() => commit(i)}
              >
                <span>{c.label}</span>
                {c.hint && <span className="ml-4 truncate font-mono text-[11px] text-muted">{c.hint}</span>}
              </button>
            </li>
          ))}
        </ul>
        <div className="flex items-center gap-3 border-t border-edge px-4 py-2 text-[11px] text-muted">
          <span><Kbd>↑↓</Kbd> navigate</span>
          <span><Kbd>↵</Kbd> run</span>
          <span><Kbd>esc</Kbd> close</span>
        </div>
      </div>
    </div>
  );
}
