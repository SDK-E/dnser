export interface Ports { dns: number; http: number; https: number; ui: number }
export interface Settings { tld: string; bind: string; upstreams: string[]; ports: Ports }
export interface Status {
  version: string; tld: string; bind: string; dns_port: number;
  ports: Ports; upstreams: string[]; dashboard_url: string; projects: number;
}
export interface Record {
  type: string; name: string; value: string;
  ttl?: number; priority?: number; weight?: number; port?: number;
}
export interface Health { up: boolean; latency_ms: number; checked_at: string; fail_count: number }
export interface Project {
  domain: string; port: number; wildcard: boolean; https: boolean;
  aliases?: string[]; records?: Record[]; health?: Health;
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
  createProject: (p: { domain: string; port: number; wildcard: boolean; https: boolean }) =>
    req<Project>("/api/v1/projects", { method: "POST", body: JSON.stringify(p) }),
  updateProject: (domain: string, patch: Partial<Pick<Project, "port" | "wildcard" | "https">>) =>
    req<Project>(`/api/v1/projects/${domain}`, { method: "PUT", body: JSON.stringify(patch) }),
  deleteProject: (domain: string) =>
    req(`/api/v1/projects/${domain}`, { method: "DELETE" }),
  addRecord: (domain: string, r: Record) =>
    req(`/api/v1/projects/${domain}/records`, { method: "POST", body: JSON.stringify(r) }),
  removeRecord: (domain: string, r: Pick<Record, "name" | "type">) =>
    req(`/api/v1/projects/${domain}/records`, { method: "DELETE", body: JSON.stringify(r) }),
};
