export interface ProjectRow {
  name: string
  domain: string
  port: number
  availability: string
  phase: string
  pid?: number
}

export interface DoctorIssue {
  kind: string
  evidence: string
  fix: string
}

export interface StatusPayload {
  daemon: { running: boolean }
  dns_port: number
  projects: ProjectRow[]
}

export interface LogLine {
  ts: string
  stream: string
  line: string
}

async function api<T>(path: string): Promise<T> {
  const token = new URLSearchParams(window.location.search).get('token') ?? ''
  const res = await fetch(path, {
    headers: { Authorization: `Bearer ${token}` },
  })
  if (!res.ok) {
    throw new Error(`${path}: ${res.status}`)
  }
  return res.json() as Promise<T>
}

export const fetchStatus = () => api<StatusPayload>('/api/status')
export const fetchDoctor = () => api<{ issues: DoctorIssue[]; count: number }>('/api/doctor')
export const fetchLogs = (project: string) =>
  api<{ lines: LogLine[] }>(`/api/logs/${encodeURIComponent(project)}`)
