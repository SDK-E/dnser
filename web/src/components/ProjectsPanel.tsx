import { useCallback, useEffect, useMemo, useState } from "react";
import { api, type AppInfo, type BackendHealth, type DotState, type Project, type Route } from "../api";
import {
  Badge, Button, Card, EmptyState, Input, Modal, Select, StatusDot,
  Tabs, useToast,
} from "./ui";

function hostOf(routeHost: string, domain: string, tld: string): string {
  if (routeHost === "@") return domain;
  if (routeHost === "*") return `*.${domain}`;
  if (routeHost.includes(".")) return routeHost.endsWith("." + tld) ? routeHost : `${routeHost}.${tld}`;
  return `${routeHost}.${domain}`;
}

export function ProjectsPanel({
  projects, apps, tld, onChanged, linkOpen, setLinkOpen,
}: {
  projects: Project[];
  apps: Record<string, AppInfo>;
  tld: string;
  onChanged: () => void;
  linkOpen: boolean;
  setLinkOpen: (open: boolean) => void;
}) {
  const [selected, setSelected] = useState<string | null>(null);

  return (
    <section className="flex min-h-0 flex-col gap-4">
      <div className="flex items-center justify-between">
        <h2 className="text-sm font-semibold tracking-wide text-muted uppercase">Projects</h2>
        <Button variant="primary" size="sm" onClick={() => setLinkOpen(true)}>+ Link project</Button>
      </div>
      {projects.length === 0 ? (
        <Card>
          <EmptyState
            title="No projects yet"
            hint="Link a folder — dnser detects the stack, picks a free port and serves it on your .test domain."
            action={<Button variant="primary" onClick={() => setLinkOpen(true)}>Link your first project</Button>}
          />
        </Card>
      ) : (
        <div className="grid min-h-0 grid-cols-1 gap-3 overflow-y-auto pr-1 sm:grid-cols-2 xl:grid-cols-3">
          {projects.map((p) => (
            <ProjectCard key={p.domain} project={p} app={apps[p.domain]} tld={tld}
              onClick={() => setSelected(p.domain)} />
          ))}
        </div>
      )}
      {selected && projects.some((p) => p.domain === selected) && (
        <ProjectDetail
          project={projects.find((p) => p.domain === selected)!}
          app={apps[selected]}
          tld={tld}
          onClose={() => setSelected(null)}
          onChanged={onChanged}
        />
      )}
      <LinkModal open={linkOpen} onClose={() => setLinkOpen(false)} tld={tld} onCreated={onChanged} />
    </section>
  );
}

function aggregate(health: BackendHealth[] | undefined): DotState {
  if (!health || health.length === 0) return "unknown";
  const ups = health.filter((h) => !h.tcp && h.up === true).length;
  const downs = health.filter((h) => !h.tcp && h.up === false).length;
  if (downs > 0 && ups === 0) return "down";
  if (ups > 0) return "up";
  return "unknown";
}

function ProjectCard({ project, app, tld, onClick }: {
  project: Project; app?: AppInfo; tld: string; onClick: () => void;
}) {
  const dot = app ? app.state : aggregate(project.backend_health);
  const tone = app ? stateToneMap[app.state] ?? "default" : "default";
  return (
    <Card interactive onClick={onClick}>
      <div className="flex items-start justify-between gap-2">
        <div className="min-w-0">
          <div className="flex items-center gap-2">
            <StatusDot state={dot as DotState} />
            <span className="truncate font-mono text-sm font-medium">{project.domain}</span>
          </div>
          <p className="mt-1 truncate text-xs text-muted">{project.path || `${project.routes?.length ?? 0} routes`}</p>
        </div>
        {app && <Badge tone={tone}>{app.state}</Badge>}
      </div>
      <div className="mt-3 flex items-center gap-2 text-[11px] text-muted">
        {app && app.port > 0 && <span>:{app.port}</span>}
        {app?.framework && <span>{app.framework}</span>}
        {(project.routes ?? []).some((r) => r.https) && <span className="text-ok">https</span>}
        {(project.routes ?? []).some((r) => r.tcp) && (
          <span>tcp :{project.routes?.find((r) => r.tcp)?.listen}</span>
        )}
        {!app && hostOf("@", project.domain, tld) && <span>{(project.records ?? []).length} records</span>}
      </div>
    </Card>
  );
}

const stateToneMap: Record<string, "green" | "red" | "amber" | "info" | "default"> = {
  up: "green", down: "red", "crash-looping": "red", starting: "amber",
  stopped: "default", failed: "info", "deps-missing": "info", unknown: "default",
};

function ProjectDetail({ project, app, tld, onClose, onChanged }: {
  project: Project; app?: AppInfo; tld: string; onClose: () => void; onChanged: () => void;
}) {
  const toast = useToast();
  const [tab, setTab] = useState<"routes" | "records" | "runner">("routes");
  const [busy, setBusy] = useState(false);

  const act = useCallback(async (fn: () => Promise<unknown>, okMsg: string) => {
    setBusy(true);
    try {
      await fn();
      toast(okMsg);
      onChanged();
    } catch (e) {
      toast(e instanceof Error ? e.message : String(e), "err");
    } finally {
      setBusy(false);
    }
  }, [onChanged, toast]);

  return (
    <Modal title={project.domain} onClose={onClose} wide>
      <div className="mb-4 flex flex-wrap items-center gap-2 text-xs text-muted">
        {project.path && <span className="font-mono">{project.path}</span>}
        {app && <Badge tone={stateToneMap[app.state] ?? "default"}>{app.state}</Badge>}
        {app?.framework && <Badge>{app.framework}</Badge>}
        {app && app.port > 0 && <Badge>: {app.port}</Badge>}
        {app && app.restarts > 0 && <Badge tone="amber">restarts: {app.restarts}</Badge>}
      </div>
      {app?.last_error && (
        <p className="mb-4 rounded-lg border border-red-500/30 bg-red-500/10 px-3 py-2 font-mono text-xs text-red-400">
          {app.last_error}
        </p>
      )}
      <Tabs
        tabs={[
          { id: "routes", label: `Routes (${project.routes?.length ?? 0})` },
          { id: "records", label: `Records (${project.records?.length ?? 0})` },
          ...(project.path ? [{ id: "runner" as const, label: "App" }] : []),
        ]}
        active={tab}
        onChange={(id) => setTab(id as typeof tab)}
      />
      <div className="mt-4 max-h-[50vh] overflow-y-auto pr-1">
        {tab === "routes" && <RoutesTab project={project} tld={tld} />}
        {tab === "records" && <RecordsTab project={project} onChanged={onChanged} act={act} busy={busy} />}
        {tab === "runner" && app !== undefined && (
          <RunnerTab app={app} hasRun={!!project.run?.command} act={act} busy={busy} />
        )}
      </div>
    </Modal>
  );
}

function RoutesTab({ project, tld }: { project: Project; tld: string }) {
  const routes = project.routes ?? [];
  const healthByBackend = new Map<string, BackendHealth>();
  for (const h of project.backend_health ?? []) healthByBackend.set(h.backend + "|" + h.host, h);
  if (routes.length === 0) return <EmptyState title="No routes" hint="This project only has DNS records." />;
  return (
    <table className="w-full text-left text-xs">
      <thead className="text-muted">
        <tr><th className="pb-2 font-medium">hostname</th><th className="pb-2 font-medium">backends</th><th className="pb-2 font-medium">tls</th></tr>
      </thead>
      <tbody className="font-mono">
        {routes.map((r: Route, i) => (
          <tr key={i} className="border-t border-edge/60 align-top">
            <td className="py-2 pr-3">
              {r.tcp ? (
                <span>: {r.listen}</span>
              ) : (
                <a
                  href={`https://${hostOf(r.host, project.domain, tld)}`}
                  target="_blank" rel="noreferrer"
                  className="hover:text-accent hover:underline"
                >
                  {hostOf(r.host, project.domain, tld)}
                </a>
              )}
              {r.force_https && <span className="ml-2 text-[10px] text-warn" title="redirects http → https">force</span>}
            </td>
            <td className="py-2 pr-3">
              <div className="flex flex-col gap-1">
                {r.backends.map((b) => {
                  const h = healthByBackend.get(b + "|" + r.host);
                  return (
                    <span key={b} className="flex items-center gap-2">
                      <StatusDot up={h?.up} />
                      <span>{r.tcp ? "tcp" : b}{h && !h.up ? " (down)" : ""}</span>
                    </span>
                  );
                })}
                {r.backends.length === 0 && <span className="text-muted">—</span>}
              </div>
            </td>
            <td className="py-2">{r.https ? "✓" : "—"}</td>
          </tr>
        ))}
      </tbody>
    </table>
  );
}

function RecordsTab({ project, onChanged, act, busy }: {
  project: Project;
  onChanged: () => void;
  act: (fn: () => Promise<unknown>, msg: string) => Promise<void>;
  busy: boolean;
}) {
  const [type, setType] = useState("A");
  const [name, setName] = useState("@");
  const [value, setValue] = useState("");
  const records = project.records ?? [];
  return (
    <div className="flex flex-col gap-3">
      <form
        className="flex flex-wrap items-center gap-2"
        onSubmit={(e) => {
          e.preventDefault();
          if (!value.trim()) return;
          void act(() => api.addRecord(project.domain, { type, name, value }), "record added").then(() => {
            setValue("");
            onChanged();
          });
        }}
      >
        <Select value={type} onChange={(e) => setType(e.target.value)} className="w-24">
          {["A", "AAAA", "CNAME", "TXT", "MX", "SRV"].map((t) => <option key={t}>{t}</option>)}
        </Select>
        <Input value={name} onChange={(e) => setName(e.target.value)} placeholder="@" className="w-28" />
        <Input value={value} onChange={(e) => setValue(e.target.value)} placeholder="value" className="flex-1" />
        <Button type="submit" variant="primary" size="sm" disabled={busy}>Add</Button>
      </form>
      {records.length === 0 ? (
        <EmptyState title="No custom records" hint="Routes already resolve automatically." />
      ) : (
        <ul className="font-mono text-xs">
          {records.map((rec, i) => (
            <li key={i} className="flex items-center justify-between border-t border-edge/60 py-2">
              <span>
                <span className="text-muted">{rec.type}</span>{" "}
                {rec.name} → {rec.value}
              </span>
              <Button size="sm" variant="danger"
                onClick={() => void act(() => api.removeRecord(project.domain, { name: rec.name, type: rec.type }), "record removed")}
                disabled={busy}
              >
                remove
              </Button>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}

function RunnerTab({ app, hasRun, act, busy }: {
  app?: AppInfo; hasRun: boolean;
  act: (fn: () => Promise<unknown>, msg: string) => Promise<void>;
  busy: boolean;
}) {
  if (!hasRun) {
    return <EmptyState title="Not managed" hint="No dev command configured for this project. Use `dnser link` with --command or a .dnser.yaml file." />;
  }
  return (
    <div className="flex flex-col gap-3">
      <div className="flex flex-wrap gap-2">
        <Button variant="primary" size="sm" disabled={busy}
          onClick={() => void act(() => api.restartApp(app?.domain ?? ""), "restarting…")}>
          Restart
        </Button>
        <Button size="sm" disabled={busy}
          onClick={() => void act(() => api.startApp(app?.domain ?? ""), "starting…")}>
          Start
        </Button>
        <Button variant="danger" size="sm" disabled={busy}
          onClick={() => void act(() => api.stopApp(app?.domain ?? ""), "stopped")}>
          Stop
        </Button>
      </div>
      <dl className="grid grid-cols-[auto_1fr] gap-x-4 gap-y-1 font-mono text-xs">
        <dt className="text-muted">command</dt><dd>{(app?.command ?? []).join(" ") || "—"}</dd>
        <dt className="text-muted">pid</dt><dd>{app?.pid ?? "—"}</dd>
        <dt className="text-muted">port</dt><dd>{app?.port ?? "—"}</dd>
        <dt className="text-muted">restarts</dt><dd>{app?.restarts ?? 0}</dd>
      </dl>
    </div>
  );
}

export function LinkModal({ open, onClose, tld, onCreated }: {
  open: boolean; onClose: () => void; tld: string; onCreated: () => void;
}) {
  const toast = useToast();
  const [path, setPath] = useState("");
  const [detecting, setDetecting] = useState(false);
  const [creating, setCreating] = useState(false);
  const [detected, setDetected] = useState<import("../api").DetectResult | null>(null);
  const [domain, setDomain] = useState("");
  const [managed, setManaged] = useState(true);
  const [wildcard, setWildcard] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!open) {
      setPath(""); setDetected(null); setDomain(""); setError(null); setManaged(true); setWildcard(true);
    }
  }, [open]);

  const doDetect = useCallback(async () => {
    if (!path.trim()) return;
    setDetecting(true);
    setError(null);
    try {
      const res = await api.detect(path.trim().replace(/^~(?=\/|$)/, ""));
      setDetected(res);
      setDomain(res.suggested_domain);
      if (res.deps_missing) setError(`dependencies missing — ${res.deps_missing}`);
    } catch (e) {
      setDetected(null);
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setDetecting(false);
    }
  }, [path]);

  const create = useCallback(async () => {
    if (!detected || !domain.trim()) return;
    setCreating(true);
    try {
      await api.createProject({
        domain: domain.trim(),
        path: detected.path,
        run: managed ? { command: (detected.recipe ?? []).join(" "), port: 0 } : undefined,
        wildcard,
        https: true,
      });
      onCreated();
      toast(`linked ${domain}. ${tld}`);
      onClose();
    } catch (e) {
      toast(e instanceof Error ? e.message : String(e), "err");
    } finally {
      setCreating(false);
    }
  }, [detected, domain, managed, wildcard, onCreated, onClose, toast, tld]);

  if (!open) return null;
  return (
    <Modal title="Link a project" onClose={onClose} wide>
      <div className="flex flex-col gap-4">
        <form
          className="flex gap-2"
          onSubmit={(e) => { e.preventDefault(); void doDetect(); }}
        >
          <Input
            value={path}
            onChange={(e) => setPath(e.target.value)}
            placeholder="/path/to/project or ~/code/my-app"
            autoFocus
          />
          <Button type="submit" variant="primary" disabled={!path.trim() || detecting}>
            {detecting ? "…" : "Detect"}
          </Button>
        </form>

        {error && detected && (
          <p className="rounded-lg border border-yellow-500/30 bg-yellow-500/10 px-3 py-2 text-xs text-warn">{error}</p>
        )}
        {error && !detected && (
          <p className="rounded-lg border border-red-500/30 bg-red-500/10 px-3 py-2 text-xs text-red-400">{error}</p>
        )}

        {detected && (
          <div className="animate-pop-in flex flex-col gap-3 rounded-xl border border-edge p-4">
            <div className="flex flex-wrap items-center gap-2 text-xs">
              {detected.framework
                ? <Badge tone="green">{detected.framework}</Badge>
                : <Badge tone="info">unknown stack</Badge>}
              {detected.recipe && detected.recipe.length > 0 && (
                <span className="font-mono text-muted">$ {detected.recipe.join(" ")}</span>
              )}
            </div>
            <label className="flex flex-col gap-1 text-xs text-muted">
              domain
              <div className="flex items-center gap-1">
                <Input value={domain} onChange={(e) => setDomain(e.target.value)} />
                <span className="text-muted">.{tld}</span>
              </div>
            </label>
            <label className="flex items-center gap-2 text-xs">
              <input type="checkbox" checked={managed} onChange={(e) => setManaged(e.target.checked)}
                className="accent-accent" />
              run the dev server for me{(detected.recipe?.length ?? 0) === 0 ? " (no command detected)" : ""}
            </label>
            <label className="flex items-center gap-2 text-xs">
              <input type="checkbox" checked={wildcard} onChange={(e) => setWildcard(e.target.checked)}
                className="accent-accent" />
              wildcard (*.domain)
            </label>
            {(detected.recipe?.length ?? 0) === 0 && managed && (
              <p className="text-xs text-muted">
                No dev command was detected. The project will be linked without a managed server —
                add a <code className="font-mono">command:</code> line to <code className="font-mono">.dnser.yaml</code> later.
              </p>
            )}
          </div>
        )}

        <div className="flex justify-end gap-2">
          <Button onClick={onClose}>Cancel</Button>
          <Button variant="primary" disabled={!detected || creating || !domain.trim()} onClick={() => void create()}>
            {creating ? "Linking…" : `Link ${domain ? domain + "." + tld : ""}`}
          </Button>
        </div>
      </div>
    </Modal>
  );
}

export function useApps(refreshKey: number): Record<string, AppInfo> {
  const [apps, setApps] = useState<Record<string, AppInfo>>({});
  const load = useCallback(async () => {
    try {
      const payload = await api.runner();
      const map: Record<string, AppInfo> = {};
      for (const a of payload.apps) map[a.domain] = a;
      setApps(map);
    } catch {
      /* daemon may be mid-restart */
    }
  }, []);
  useEffect(() => {
    void load();
    const id = setInterval(load, 3000);
    return () => clearInterval(id);
  }, [load, refreshKey]);
  return useMemo(() => apps, [apps]);
}

export function DeleteButton({ domain, onDeleted }: { domain: string; onDeleted: () => void }) {
  const toast = useToast();
  const [confirming, setConfirming] = useState(false);
  if (!confirming) {
    return <Button variant="danger" size="sm" onClick={() => setConfirming(true)}>Delete project</Button>;
  }
  return (
    <span className="flex items-center gap-2 text-xs">
      really delete?
      <Button variant="danger" size="sm" onClick={() => {
        void api.deleteProject(domain).then(() => { toast("deleted"); onDeleted(); }).catch((e) => toast(String(e), "err"));
      }}>yes</Button>
      <Button size="sm" onClick={() => setConfirming(false)}>no</Button>
    </span>
  );
}
