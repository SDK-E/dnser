import { useState } from "react";
import { api, type Project, type Record as DnsRecord } from "../api";
import { Badge, Button, Input, Modal, Select, StatusDot, Toggle } from "./ui";

interface Props {
  projects: Project[];
  onChanged: () => void;
}

export function ProjectsPanel({ projects, onChanged }: Props) {
  const [showAdd, setShowAdd] = useState(false);
  const [expanded, setExpanded] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);

  const run = async (fn: () => Promise<unknown>) => {
    try {
      await fn();
      setError(null);
      onChanged();
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    }
  };

  return (
    <section className="rounded-2xl border border-edge bg-surface">
      <header className="flex items-center justify-between px-5 py-4">
        <h2 className="text-sm font-semibold tracking-wide">Projects</h2>
        <Button variant="primary" onClick={() => setShowAdd(true)}>+ Link project</Button>
      </header>

      {error && (
        <p className="mx-5 mb-3 rounded-lg border border-red-500/30 bg-red-500/10 px-3 py-2 text-xs text-red-400">
          {error}
        </p>
      )}

      {projects.length === 0 ? (
        <p className="px-5 pb-8 pt-2 text-center text-sm text-muted">
          No projects linked yet. Click “Link project” or run{" "}
          <code className="rounded bg-raised px-1.5 py-0.5 font-mono text-xs">dnser link</code>.
        </p>
      ) : (
        <ul className="divide-y divide-edge">
          {projects.map((p) => (
            <li key={p.domain} className="px-5 py-3">
              <div className="flex items-center justify-between gap-3">
                <button
                  className="flex min-w-0 items-center gap-3 text-left"
                  onClick={() => setExpanded(expanded === p.domain ? null : p.domain)}
                >
                  <StatusDot up={p.health?.up} />
                  <span className="truncate font-mono text-sm">{p.domain}</span>
                  {p.wildcard && <Badge>✳ wildcard</Badge>}
                  {p.https && <Badge tone="green">https</Badge>}
                </button>
                <div className="flex shrink-0 items-center gap-3">
                  <span className="font-mono text-xs text-muted">:{p.port || "—"}</span>
                  {p.health && (
                    <span className="font-mono text-xs text-muted">{p.health.latency_ms}ms</span>
                  )}
                  <Button variant="danger" onClick={() => run(() => api.deleteProject(p.domain))}>
                    Unlink
                  </Button>
                </div>
              </div>

              {expanded === p.domain && (
                <RecordEditor project={p} onDone={() => onChanged()} />
              )}
            </li>
          ))}
        </ul>
      )}

      {showAdd && (
        <AddProjectModal
          onClose={() => setShowAdd(false)}
          onCreate={(req) => run(async () => { await api.createProject(req); setShowAdd(false); })}
        />
      )}
    </section>
  );
}

function AddProjectModal({ onClose, onCreate }: {
  onClose: () => void;
  onCreate: (req: { domain: string; port: number; wildcard: boolean; https: boolean }) => void;
}) {
  const [domain, setDomain] = useState("");
  const [port, setPort] = useState("3000");
  const [wildcard, setWildcard] = useState(true);
  const [https, setHttps] = useState(true);

  return (
    <Modal title="Link project" onClose={onClose}>
      <form
        className="space-y-4"
        onSubmit={(e) => {
          e.preventDefault();
          onCreate({ domain, port: Number(port) || 0, wildcard, https });
        }}
      >
        <div>
          <label className="mb-1 block text-xs text-muted">Domain</label>
          <Input autoFocus placeholder="myproject" value={domain}
            onChange={(e) => setDomain(e.target.value)} required />
          <p className="mt-1 text-[11px] text-muted">.test is appended automatically</p>
        </div>
        <div>
          <label className="mb-1 block text-xs text-muted">Local port</label>
          <Input type="number" min={0} max={65535} value={port}
            onChange={(e) => setPort(e.target.value)} />
        </div>
        <Toggle checked={wildcard} onChange={setWildcard} label="Resolve all subdomains (*.domain)" />
        <Toggle checked={https} onChange={setHttps} label="HTTPS reverse proxy" />
        <div className="flex justify-end gap-2 pt-2">
          <Button onClick={onClose}>Cancel</Button>
          <Button variant="primary" type="submit">Link</Button>
        </div>
      </form>
    </Modal>
  );
}

const RECORD_TYPES = ["A", "AAAA", "CNAME", "TXT", "MX", "SRV", "NS"];

function RecordEditor({ project, onDone }: { project: Project; onDone: () => void }) {
  const [type, setType] = useState("TXT");
  const [name, setName] = useState("@");
  const [value, setValue] = useState("");
  const records = project.records ?? [];

  const add = async () => {
    if (!value.trim()) return;
    try {
      await api.addRecord(project.domain, { type, name, value: value.trim() });
      setValue("");
      onDone();
    } catch { /* surfaced on refresh */ }
  };

  const remove = async (r: DnsRecord) => {
    try {
      await api.removeRecord(project.domain, { name: r.name, type: r.type });
      onDone();
    } catch { /* surfaced on refresh */ }
  };

  return (
    <div className="mt-3 rounded-xl border border-edge bg-base p-4">
      <table className="w-full text-left font-mono text-xs">
        <tbody>
          {records.length === 0 && (
            <tr><td colSpan={3} className="py-2 text-muted">No extra records — implicit A records active.</td></tr>
          )}
          {records.map((r, i) => (
            <tr key={`${r.type}-${r.name}-${i}`} className="border-b border-edge/50 last:border-0">
              <td className="w-16 py-1.5 pr-2 text-accent">{r.type}</td>
              <td className="py-1.5 pr-2 text-muted">{r.name}</td>
              <td className="py-1.5 pr-2 break-all">{r.value}</td>
              <td className="w-6 py-1.5 text-right">
                <button className="text-muted hover:text-red-400" onClick={() => remove(r)}>✕</button>
              </td>
            </tr>
          ))}
        </tbody>
      </table>

      <div className="mt-3 flex flex-wrap items-center gap-2">
        <Select value={type} onChange={(e) => setType(e.target.value)}>
          {RECORD_TYPES.map((t) => <option key={t}>{t}</option>)}
        </Select>
        <Input className="!w-28" value={name} onChange={(e) => setName(e.target.value)} placeholder="@" />
        <Input className="min-w-40 flex-1" value={value} onChange={(e) => setValue(e.target.value)}
          placeholder="value" onKeyDown={(e) => e.key === "Enter" && add()} />
        <Button variant="primary" onClick={add}>Add record</Button>
      </div>
      <p className="mt-2 text-[11px] text-muted">
        Records live at <span className="font-mono">name.project.tld</span>; “@” targets the apex.
      </p>
    </div>
  );
}
