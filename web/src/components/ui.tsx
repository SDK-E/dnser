import { type ReactNode } from "react";

export function Badge({ children, tone = "default" }: { children: ReactNode; tone?: "default" | "green" | "red" | "amber" }) {
  const tones = {
    default: "border-edge bg-raised text-muted",
    green: "border-accent/40 bg-accent/10 text-accent",
    red: "border-red-500/40 bg-red-500/10 text-red-400",
    amber: "border-amber-500/40 bg-amber-500/10 text-amber-400",
  };
  return (
    <span className={`inline-flex items-center gap-1 rounded-md border px-2 py-0.5 text-[11px] font-medium ${tones[tone]}`}>
      {children}
    </span>
  );
}

export function StatusDot({ up }: { up?: boolean }) {
  if (up === undefined) return <span className="inline-block h-2 w-2 rounded-full bg-edge" title="unknown" />;
  return (
    <span
      className={`inline-block h-2 w-2 rounded-full ${up ? "bg-accent shadow-[0_0_6px] shadow-accent/60" : "bg-red-500 shadow-[0_0_6px] shadow-red-500/60"}`}
      title={up ? "upstream healthy" : "upstream down"}
    />
  );
}

export function Button({ children, onClick, variant = "ghost", type = "button", disabled }: {
  children: ReactNode; onClick?: () => void; variant?: "primary" | "ghost" | "danger";
  type?: "button" | "submit"; disabled?: boolean;
}) {
  const styles = {
    primary: "bg-accent text-base hover:bg-accent-dim font-semibold",
    ghost: "border border-edge bg-raised hover:border-accent/50 text-ink",
    danger: "border border-red-500/40 text-red-400 hover:bg-red-500/10",
  };
  return (
    <button
      type={type}
      onClick={onClick}
      disabled={disabled}
      className={`rounded-lg px-3 py-1.5 text-sm transition-colors disabled:opacity-40 ${styles[variant]}`}
    >
      {children}
    </button>
  );
}

export function Input(props: React.InputHTMLAttributes<HTMLInputElement>) {
  return (
    <input
      {...props}
      className="w-full rounded-lg border border-edge bg-base px-3 py-1.5 text-sm text-ink placeholder-muted/60 outline-none focus:border-accent/60"
    />
  );
}

export function Select(props: React.SelectHTMLAttributes<HTMLSelectElement>) {
  return (
    <select
      {...props}
      className="rounded-lg border border-edge bg-base px-3 py-1.5 text-sm text-ink outline-none focus:border-accent/60"
    />
  );
}

export function Toggle({ checked, onChange, label }: { checked: boolean; onChange: (v: boolean) => void; label: string }) {
  return (
    <label className="flex cursor-pointer items-center gap-2 text-sm text-muted">
      <button
        type="button"
        onClick={() => onChange(!checked)}
        className={`relative h-5 w-9 rounded-full transition-colors ${checked ? "bg-accent" : "bg-edge"}`}
      >
        <span
          className={`absolute top-0.5 h-4 w-4 rounded-full bg-base transition-all ${checked ? "left-[18px]" : "left-0.5"}`}
        />
      </button>
      {label}
    </label>
  );
}

export function Modal({ title, onClose, children }: { title: string; onClose: () => void; children: ReactNode }) {
  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 p-4" onClick={onClose}>
      <div
        className="w-full max-w-md rounded-2xl border border-edge bg-surface p-6 shadow-2xl"
        onClick={(e) => e.stopPropagation()}
      >
        <div className="mb-4 flex items-center justify-between">
          <h2 className="text-base font-semibold">{title}</h2>
          <button onClick={onClose} className="text-muted hover:text-ink">✕</button>
        </div>
        {children}
      </div>
    </div>
  );
}
