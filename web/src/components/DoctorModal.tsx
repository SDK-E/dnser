import { useCallback, useEffect, useState } from "react";
import { api, type DoctorPayload } from "../api";
import { Button, Modal, Spinner } from "./ui";

const mark: Record<string, string> = { ok: "✓", warn: "!", fail: "✗" };
const tone: Record<string, string> = {
  ok: "text-ok", warn: "text-warn", fail: "text-err",
};

export function DoctorModal({ open, onClose }: { open: boolean; onClose: () => void }) {
  const [data, setData] = useState<DoctorPayload | null>(null);
  const [error, setError] = useState<string | null>(null);

  const load = useCallback(async () => {
    setError(null);
    try {
      setData(await api.doctor());
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    }
  }, []);

  useEffect(() => {
    if (open) void load();
  }, [open, load]);

  if (!open) return null;
  return (
    <Modal title="Doctor — system diagnostics" onClose={onClose} wide>
      {error && <p className="text-sm text-red-400">{error}</p>}
      {!data && !error && (
        <div className="flex items-center gap-2 text-sm text-muted"><Spinner /> running checks…</div>
      )}
      {data && (
        <div className="flex flex-col gap-2">
          <ul className="flex flex-col gap-2">
            {data.checks.map((c) => (
              <li key={c.name} className="flex items-start gap-3 rounded-lg border border-edge/60 px-3 py-2 text-sm">
                <span className={`font-bold ${tone[c.status] ?? ""}`}>{mark[c.status] ?? "?"}</span>
                <span>
                  <span className="font-mono text-xs text-muted">{c.name}</span>
                  <br />
                  {c.detail}
                </span>
              </li>
            ))}
          </ul>
          <div className="flex justify-end pt-2">
            <Button size="sm" onClick={() => void load()}>Re-run checks</Button>
          </div>
        </div>
      )}
    </Modal>
  );
}
