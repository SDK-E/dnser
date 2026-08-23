import { useCallback, useEffect, useState } from "react";
import { api, type Project, type Status } from "./api";
import { ProjectsPanel } from "./components/ProjectsPanel";
import { LogsPanel } from "./components/LogsPanel";

export default function App() {
  const [status, setStatus] = useState<Status | null>(null);
  const [projects, setProjects] = useState<Project[]>([]);
  const [error, setError] = useState<string | null>(null);

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
        <div className="text-right text-xs text-muted">
          {status ? (
            <>
              <span className="font-mono text-ink">v{status.version}</span>
              <br />
              <a href={`http://${status.bind}:${status.ports.ui}`} className="hover:text-accent">
                {status.dashboard_url}
              </a>
            </>
          ) : (
            "connecting…"
          )}
        </div>
      </header>

      {error && (
        <div className="mb-4 rounded-xl border border-red-500/30 bg-red-500/10 px-4 py-3 text-sm text-red-400">
          Cannot reach daemon API — is <code className="font-mono">dnser start</code> running? ({error})
        </div>
      )}

      <main className="grid min-h-0 flex-1 grid-cols-1 gap-6 lg:grid-cols-[1fr_420px]">
        <ProjectsPanel projects={projects} onChanged={refresh} />
        <LogsPanel />
      </main>
    </div>
  );
}
