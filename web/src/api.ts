export interface Ports { dns: number; http: number; https: number; ui: number }
export interface Settings {
  tld: string; bind: string; upstreams: string[]; autostart?: boolean; ports: Ports;
  force_https?: boolean; path_refresh_minutes?: number;
}
export interface Status {
  version: string; tld: string; bind: string; dns_port: number;
  ports: Ports; upstreams: string[]; dashboard_url: string; projects: number;
}
export interface Record {
  type: string; name: string; value: string;
  ttl?: number; priority?: number; weight?: number; port?: number;
}
export interface Route {
  host: string; backends: string[]; tcp?: boolean; udp?: boolean; listen?: number;
  https?: boolean; force_https?: boolean; paths?: string[];
}
export interface Service {
  name: string; type?: string; command?: string; host?: string;
  port?: number; transport?: "" | "tcp" | "udp"; dns?: boolean;
}
export interface RunConfig { command?: string; mode?: string; port?: number }
export interface BackendHealth {
  backend: string; host: string; tcp?: boolean;
  up?: boolean; latency_ms?: number; checked_at?: string; fail_count?: number;
}
export interface DnsRecord {
  type: string; name: string; value: string;
  ttl?: number; priority?: number; weight?: number; port?: number;
}
export type Project = {
  domain: string; path?: string; run?: RunConfig; services?: Service[];
  routes?: Route[]; records?: DnsRecord[]; created_at?: string;
  backend_health?: BackendHealth[];
};
export type DotState = "unknown" | "up" | "down" | "starting" | "crash-looping" | "stopped";
export interface AppInfo {
  domain: string; path: string; framework: string; state: DotState | "failed" | "" ;
  port: number; pid?: number; command?: string[];
  last_error?: string; restarts: number; started_at?: string;
}
export interface RunnerPayload { apps: AppInfo[]; deps_missing: { [domain: string]: string } }
export interface DoctorCheck { name: string; status: "ok" | "warn" | "fail"; detail: string }
export interface DoctorPayload { status: string; checks: DoctorCheck[] }
export interface DetectResult {
  path: string; framework?: string; recipe?: string[]; port_env: boolean;
  deps_missing?: string; suggested_domain: string;
}
export interface LogEvent {
  time: string; name: string; type: string; source: string; answer: string; latency_ns: number;
}
export interface DesktopStatus {
  status: Status; setup: SetupStatusView; autostart: boolean; update: UpdateInfo;
}
export interface SetupStatusView {
  ca_trusted: boolean; ca_trust_mode?: string; routed: boolean; routing_mode: string;
  resolver_domains?: string[]; dns_port: number; needs_port_53: boolean;
}
export interface SetupStep { name: string; detail?: string; err?: string }
export interface UpdateInfo { available: boolean; version?: string; url?: string }

async function req<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(path, {
    headers: init?.body ? { "Content-Type": "application/json" } : undefined,
    ...init,
  });
  if (!res.ok) {
    const body = await res.json().catch(() => ({ error: res.statusText }));
    throw new Error(body.error ?? `${res.status}`);
  }
  return res.json() as Promise<T>;
}

export const desktop = {
  status: async (): Promise<DesktopStatus | null> => {
    try {
      return await req<DesktopStatus>("/api/v1/desktop/status");
    } catch (e) {
      if (e instanceof TypeError) throw e;
      return null;
    }
  },
  runSetup: () => req<{ steps: SetupStep[] }>("/api/v1/desktop/setup", { method: "POST" }),
  revert: () => req<{ ok: string }>("/api/v1/desktop/revert", { method: "POST" }),
  setAutostart: (enabled: boolean) =>
    req<{ enabled: boolean }>("/api/v1/desktop/autostart", { method: "POST", body: JSON.stringify({ enabled }) }),
};

export const api = {
  status: () => req<Status>("/api/v1/status"),
  projects: () => req<{ projects: Project[] }>("/api/v1/projects"),
  runner: () => req<RunnerPayload>("/api/v1/runner"),
  doctor: () => req<DoctorPayload>("/api/v1/doctor"),
  detect: (path: string) =>
    req<DetectResult>("/api/v1/detect", { method: "POST", body: JSON.stringify({ path }) }),
  createProject: (p: {
    domain: string;
    routes?: Route[];
    path?: string;
    run?: RunConfig;
    port?: number;
    wildcard?: boolean;
    https?: boolean;
  }) => req<Project>("/api/v1/projects", { method: "POST", body: JSON.stringify(p) }),
  updateProject: (
    domain: string,
    patch: Partial<{
      routes: Route[];
      services: Service[];
      path: string;
      run: RunConfig | null;
    }>,
  ) => req<Project>(`/api/v1/projects/${domain}`, { method: "PUT", body: JSON.stringify(patch) }),
  settings: () => req<Settings>("/api/v1/settings"),
  updateSettings: (patch: Partial<{
    force_https: boolean;
    path_refresh_minutes: number;
    autostart: boolean;
    bind: string;
    upstreams: string[];
    ports: Partial<Ports>;
  }>) => req<Settings>("/api/v1/settings", { method: "PUT", body: JSON.stringify(patch) }),
  deleteProject: (domain: string) =>
    req(`/api/v1/projects/${domain}`, { method: "DELETE" }),
  addRecord: (domain: string, r: Record) =>
    req(`/api/v1/projects/${domain}/records`, { method: "POST", body: JSON.stringify(r) }),
  removeRecord: (domain: string, r: Pick<Record, "name" | "type">) =>
    req(`/api/v1/projects/${domain}/records`, { method: "DELETE", body: JSON.stringify(r) }),
  restartApp: (domain: string) => req<AppInfo>(`/api/v1/runner/action/restart/${domain}`, { method: "POST" }),
  stopApp: (domain: string) => req<AppInfo>(`/api/v1/runner/action/stop/${domain}`, { method: "POST" }),
  startApp: (domain: string) => req<AppInfo>(`/api/v1/runner/action/start/${domain}`, { method: "POST" }),
};

export function projectURLs(p: Project, tld: string): string[] {
  const urls: string[] = [];
  for (const r of p.routes ?? []) {
    if (r.tcp || !r.https) continue;
    const host = hostOf(r.host, p.domain, tld);
    urls.push(`https://${host}`);
  }
  return urls.length ? urls : [`https://${p.domain}`];
}

function hostOf(routeHost: string, domain: string, tld: string): string {
  if (routeHost === "@") return domain;
  if (routeHost === "*") return `*.${domain}`;
  if (routeHost.includes(".")) return routeHost.endsWith("." + tld) ? routeHost : `${routeHost}.${tld}`;
  return `${routeHost}.${domain}`;
}

export function stateTone(state: string | undefined): "ok" | "err" | "warn" | "info" | "default" {
  switch (state) {
    case "up": return "ok";
    case "crash-looping": return "err";
    case "starting": return "warn";
    case "stopped": return "default";
    case "deps-missing":
    case "failed": return "info";
    default: return "default";
  }
}
