import { useCallback, useEffect, useState } from "react";
import { api, desktop, type Project, type Status } from "./api";
import { ProjectsPanel, useApps } from "./components/ProjectsPanel";
import { DoctorModal } from "./components/DoctorModal";
import { Palette } from "./components/Palette";
import { LogsPanel } from "./components/LogsPanel";
import { DesktopPanel } from "./components/DesktopPanel";
import { SettingsModal } from "./components/SettingsModal";
import { Button, Kbd } from "./components/ui";

export default function App() {
  const [status, setStatus] = useState<Status | null>(null);
  const [projects, setProjects] = useState<Project[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [refreshKey, setRefreshKey] = useState(0);
  const [doctorOpen, setDoctorOpen] = useState(false);
  const [linkOpen, setLinkOpen] = useState(false);
  const [settingsOpen, setSettingsOpen] = useState(false);
  const [paletteOpen, setPaletteOpen] = useState(false);
  const [tourDismissed, setTourDismissed] = useState(
    () => localStorage.getItem("dnser.tour") === "done",
  );
  const apps = useApps(refreshKey);

  const refresh = useCallback(async () => {
    try {
      const [s, p] = await Promise.all([api.status(), api.projects()]);
      setStatus(s);
      setProjects(p.projects);
      setError(null);
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    }
  }, []);

  useEffect(() => {
    refresh();
    const id = setInterval(refresh, 4000);
    return () => clearInterval(id);
  }, [refresh]);

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === "k") {
        e.preventDefault();
        setPaletteOpen((v) => !v);
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, []);

  const onChanged = useCallback(() => {
    setRefreshKey((k) => k + 1);
    void refresh();
  }, [refresh]);

  return (
    <div className="mx-auto flex h-screen max-w-7xl flex-col p-6">
      <header className="mb-6 flex items-end justify-between">
        <div>
          <h1 className="text-xl font-bold tracking-tight">
            DNS<span className="text-accent">.</span>er
            <span className="ml-3 align-middle font-mono text-xs font-normal text-muted">dashboard</span>
          </h1>
          {status && (
            <p className="mt-1 text-xs text-muted">
              dns :{status.dns_port} · proxy :{status.ports.http}/:{status.ports.https} · tld .
              {status.tld} · upstream {status.upstreams[0]}
            </p>
          )}
        </div>
        <div className="flex items-center gap-3 text-right text-xs text-muted">
          <button
            onClick={() => setPaletteOpen(true)}
            className="hidden items-center gap-2 rounded-lg border border-edge px-2.5 py-1.5 transition-colors hover:border-accent/40 hover:text-ink sm:flex"
          >
            <span>Search</span>
            <Kbd>⌘K</Kbd>
          </button>
          <Button size="sm" onClick={() => setDoctorOpen(true)}>Doctor</Button>
          <Button size="sm" onClick={() => setSettingsOpen(true)}>Settings</Button>
          {status && (
            <div>
              <span className="font-mono text-ink">v{status.version}</span>
              <br />
              <a href={`http://${status.bind}:${status.ports.ui}`} className="hover:text-accent">
                {status.dashboard_url}
              </a>
            </div>
          )}
        </div>
      </header>

      {error && (
        <div className="mb-4 rounded-xl border border-red-500/30 bg-red-500/10 px-4 py-3 text-sm text-red-400">
          Cannot reach daemon API — is <code className="font-mono">dnser start</code> running? ({error})
        </div>
      )}

      {!tourDismissed && !error && (
        <div className="animate-slide-in-right mb-4 flex items-start justify-between gap-4 rounded-xl border border-accent/30 bg-accent/10 px-4 py-3 text-sm">
          <p>
            <strong>Welcome.</strong> Point any folder at DNSer with{" "}
            <code className="font-mono text-accent">dnser link ~/path/to/app</code> — it detects the
            stack, picks a free port, and serves it on <em>*.{status?.tld ?? "test"}</em>. Or use the
            button in the Projects panel.
          </p>
          <button
            className="shrink-0 text-xs text-muted hover:text-ink"
            onClick={() => {
              localStorage.setItem("dnser.tour", "done");
              setTourDismissed(true);
            }}
          >
            got it
          </button>
        </div>
      )}

      <DesktopPanel onChanged={onChanged} />

      <main className="grid min-h-0 flex-1 grid-cols-1 gap-6 lg:grid-cols-[1fr_420px]">
        <ProjectsPanel
          projects={projects}
          apps={apps}
          tld={status?.tld ?? "test"}
          onChanged={onChanged}
          linkOpen={linkOpen}
          setLinkOpen={setLinkOpen}
        />
        <LogsPanel />
      </main>

      <DoctorModal open={doctorOpen} onClose={() => setDoctorOpen(false)} />
      <SettingsModal open={settingsOpen} onClose={() => setSettingsOpen(false)} />
      <Palette
        open={paletteOpen}
        onClose={() => setPaletteOpen(false)}
        projects={projects}
        apps={apps}
        tld={status?.tld ?? "test"}
        onOpenDoctor={() => setDoctorOpen(true)}
        onOpenLink={() => setLinkOpen(true)}
        onOpenProject={(domain) =>
          window.open(`https://${domain}`, "_blank", "noopener")
        }
      />
    </div>
  );
}

export type { Project };
export { desktop };
