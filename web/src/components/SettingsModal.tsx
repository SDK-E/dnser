import { useCallback, useEffect, useState } from "react";
import { api, type Settings } from "../api";
import { Button, Input, Modal, useToast } from "./ui";

export function SettingsModal({ open, onClose }: { open: boolean; onClose: () => void }) {
  const toast = useToast();
  const [draft, setDraft] = useState<Settings | null>(null);
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    if (!open) return;
    api.settings().then(setDraft).catch((e) => toast(String(e), "err"));
  }, [open, toast]);

  const save = useCallback(async () => {
    if (!draft) return;
    setSaving(true);
    try {
      await api.updateSettings({
        force_https: draft.force_https,
        path_refresh_minutes: draft.path_refresh_minutes,
        autostart: draft.autostart,
        ports: draft.ports,
      });
      toast("settings saved — daemon hot-reloads");
      onClose();
    } catch (e) {
      toast(e instanceof Error ? e.message : String(e), "err");
    } finally {
      setSaving(false);
    }
  }, [draft, onClose, toast]);

  if (!open || !draft) return null;
  const num = (v: string): number | "" => (v === "" ? "" : Number(v));
  return (
    <Modal title="Settings" onClose={onClose} wide>
      <div className="flex flex-col gap-4 text-xs">
        <div className="grid grid-cols-2 gap-3">
          <label className="flex items-center gap-2">
            <input
              type="checkbox" checked={!!draft.force_https}
              onChange={(e) => setDraft({ ...draft, force_https: e.target.checked })}
              className="accent-accent"
            />
            <span>
              force HTTPS
              <span className="block text-muted">redirect plain HTTP on every HTTPS route</span>
            </span>
          </label>
          <label className="flex items-center gap-2">
            <input
              type="checkbox" checked={draft.autostart ?? false}
              disabled
              className="accent-accent opacity-50"
            />
            <span>
              autostart daemon
              <span className="block text-muted">read-only here; toggle via Desktop panel</span>
            </span>
          </label>
        </div>

        <label className="flex flex-col gap-1">
          managed-command PATH refresh (minutes)
          <Input
            type="number"
            value={draft.path_refresh_minutes ?? 1440}
            onChange={(e) => setDraft({ ...draft, path_refresh_minutes: num(e.target.value) as number })}
            className="w-40 font-mono"
          />
          <span className="text-muted">
            how often the daemon re-reads the login-shell PATH used to launch dev servers & services (0 = default 1440)
          </span>
        </label>

        <fieldset className="rounded-lg border border-edge p-3">
          <legend className="px-1 text-muted">listener ports</legend>
          <div className="grid grid-cols-4 gap-2">
            {(["dns", "http", "https", "ui"] as const).map((k) => (
              <label key={k} className="flex flex-col gap-1">
                {k}
                <Input
                  type="number"
                  value={draft.ports[k]}
                  onChange={(e) =>
                    setDraft({
                      ...draft,
                      ports: { ...draft.ports, [k]: e.target.value === "" ? draft.ports[k] : Number(e.target.value) },
                    })
                  }
                  className="font-mono"
                />
              </label>
            ))}
          </div>
        </fieldset>

        <div className="flex justify-end gap-2">
          <Button onClick={onClose}>Cancel</Button>
          <Button variant="primary" disabled={saving} onClick={() => void save()}>
            {saving ? "Saving…" : "Save settings"}
          </Button>
        </div>
      </div>
    </Modal>
  );
}
