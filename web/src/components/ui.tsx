import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useRef,
  useState,
  type ReactNode,
} from "react";

/* ------------------------------------------------------------------ */
/* design tokens (mirrors index.css @theme; single source for TS)      */
/* ------------------------------------------------------------------ */

export const tokens = {
  color: {
    base: "#071b03",
    surface: "#0a2705",
    raised: "#0e3309",
    overlay: "#123f0c",
    edge: "#1c4a14",
    edgeStrong: "#2a6420",
    accent: "#2cdb16",
    accentDim: "#1f9410",
    ink: "#e6f4e2",
    muted: "#93ad8f",
    faint: "#5d7a5b",
    ok: "#2cdb16",
    warn: "#ffb224",
    err: "#ff5d47",
    info: "#4cc9f0",
  },
  ease: "cubic-bezier(0.16, 1, 0.3, 1)",
} as const;

/* ------------------------------------------------------------------ */
/* Badge                                                               */
/* ------------------------------------------------------------------ */

type Tone = "default" | "green" | "red" | "amber" | "info";

const badgeTones: Record<Tone, string> = {
  default: "border-edge bg-raised text-muted",
  green: "border-accent/40 bg-accent/10 text-accent",
  red: "border-err/40 bg-err/10 text-err",
  amber: "border-warn/40 bg-warn/10 text-warn",
  info: "border-info/40 bg-info/10 text-info",
};

export function Badge({
  children,
  tone = "default",
}: {
  children: ReactNode;
  tone?: Tone;
}) {
  return (
    <span
      className={`inline-flex items-center gap-1 rounded-md border px-2 py-0.5 text-[11px] font-medium ${badgeTones[tone]}`}
    >
      {children}
    </span>
  );
}

/* ------------------------------------------------------------------ */
/* StatusDot                                                           */
/* ------------------------------------------------------------------ */

export type DotState =
  | "unknown"
  | "up"
  | "down"
  | "starting"
  | "crash-looping"
  | "stopped";

const dotStyles: Record<DotState, { cls: string; title: string }> = {
  unknown: { cls: "bg-edge", title: "unknown" },
  up: { cls: "bg-ok shadow-[0_0_6px] shadow-ok/60", title: "healthy" },
  down: { cls: "bg-err shadow-[0_0_6px] shadow-err/60", title: "down" },
  starting: { cls: "bg-warn animate-dot-pulse text-warn", title: "starting…" },
  "crash-looping": { cls: "bg-err animate-dot-shake", title: "crash-looping" },
  stopped: { cls: "bg-faint", title: "stopped" },
};

export function StatusDot({ up, state }: { up?: boolean; state?: DotState }) {
  const resolved: DotState = state ?? (up === undefined ? "unknown" : up ? "up" : "down");
  const style = dotStyles[resolved];
  return (
    <span
      className={`inline-block h-2 w-2 shrink-0 rounded-full ${style.cls}`}
      title={style.title}
    />
  );
}

/* ------------------------------------------------------------------ */
/* Button                                                              */
/* ------------------------------------------------------------------ */

export function Button({
  children,
  onClick,
  variant = "ghost",
  size = "md",
  type = "button",
  disabled,
  title,
}: {
  children: ReactNode;
  onClick?: () => void;
  variant?: "primary" | "ghost" | "danger" | "subtle";
  size?: "sm" | "md";
  type?: "button" | "submit";
  disabled?: boolean;
  title?: string;
}) {
  const variants = {
    primary:
      "bg-accent text-base hover:bg-accent-dim font-semibold shadow-[0_1px_8px_rgba(44,219,22,0.25)] active:shadow-none",
    ghost: "border border-edge bg-raised hover:border-accent/50 text-ink",
    danger: "border border-err/40 text-err hover:bg-err/10",
    subtle: "text-muted hover:text-ink hover:bg-raised",
  };
  const sizes = { sm: "px-2 py-1 text-xs", md: "px-3 py-1.5 text-sm" };
  return (
    <button
      type={type}
      onClick={onClick}
      disabled={disabled}
      title={title}
      className={`rounded-lg transition-all duration-150 active:scale-[0.98] disabled:pointer-events-none disabled:opacity-40 ${variants[variant]} ${sizes[size]}`}
    >
      {children}
    </button>
  );
}

/* ------------------------------------------------------------------ */
/* Inputs                                                              */
/* ------------------------------------------------------------------ */

const fieldCls =
  "w-full rounded-lg border border-edge bg-base px-3 py-1.5 text-sm text-ink placeholder-muted/60 outline-none transition-colors focus:border-accent/60";

export function Input(props: React.InputHTMLAttributes<HTMLInputElement>) {
  return <input {...props} className={`${fieldCls} ${props.className ?? ""}`} />;
}

export function Select(props: React.SelectHTMLAttributes<HTMLSelectElement>) {
  return (
    <select {...props} className={`${fieldCls} ${props.className ?? ""}`} />
  );
}

export function Toggle({
  checked,
  onChange,
  label,
}: {
  checked: boolean;
  onChange: (v: boolean) => void;
  label: string;
}) {
  return (
    <label className="flex cursor-pointer items-center gap-2 text-sm text-muted">
      <button
        type="button"
        onClick={() => onChange(!checked)}
        className={`relative h-5 w-9 rounded-full transition-colors duration-200 ${
          checked ? "bg-accent" : "bg-edge"
        }`}
      >
        <span
          className={`absolute top-0.5 h-4 w-4 rounded-full bg-base transition-all duration-200 ${
            checked ? "left-[18px]" : "left-0.5"
          }`}
        />
      </button>
      {label}
    </label>
  );
}

/* ------------------------------------------------------------------ */
/* Card                                                                */
/* ------------------------------------------------------------------ */

export function Card({
  children,
  onClick,
  interactive,
}: {
  children: ReactNode;
  onClick?: () => void;
  interactive?: boolean;
}) {
  return (
    <div
      onClick={onClick}
      className={`animate-pop-in rounded-[var(--radius-card)] border border-edge bg-surface p-4 ${
        interactive
          ? "cursor-pointer transition-all duration-150 hover:-translate-y-px hover:border-accent/40 hover:shadow-[0_4px_16px_rgba(0,0,0,0.35)]"
          : ""
      }`}
    >
      {children}
    </div>
  );
}

/* ------------------------------------------------------------------ */
/* Modal                                                               */
/* ------------------------------------------------------------------ */

export function Modal({
  title,
  onClose,
  children,
  wide,
}: {
  title: string;
  onClose: () => void;
  children: ReactNode;
  wide?: boolean;
}) {
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => e.key === "Escape" && onClose();
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [onClose]);
  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 p-4 animate-fade-in"
      onClick={onClose}
    >
      <div
        className={`w-full ${wide ? "max-w-2xl" : "max-w-md"} rounded-2xl border border-edge bg-surface p-6 shadow-2xl animate-pop-in`}
        onClick={(e) => e.stopPropagation()}
      >
        <div className="mb-4 flex items-center justify-between">
          <h2 className="text-base font-semibold">{title}</h2>
          <button
            onClick={onClose}
            className="text-muted transition-colors hover:text-ink"
          >
            ✕
          </button>
        </div>
        {children}
      </div>
    </div>
  );
}

/* ------------------------------------------------------------------ */
/* Tabs                                                                */
/* ------------------------------------------------------------------ */

export function Tabs<T extends string>({
  tabs,
  active,
  onChange,
}: {
  tabs: { id: T; label: string }[];
  active: T;
  onChange: (id: T) => void;
}) {
  return (
    <div className="flex gap-1 rounded-lg border border-edge bg-base p-1">
      {tabs.map((t) => (
        <button
          key={t.id}
          onClick={() => onChange(t.id)}
          className={`flex-1 rounded-md px-3 py-1 text-sm transition-colors ${
            active === t.id
              ? "bg-raised font-medium text-ink"
              : "text-muted hover:text-ink"
          }`}
        >
          {t.label}
        </button>
      ))}
    </div>
  );
}

/* ------------------------------------------------------------------ */
/* Empty state                                                         */
/* ------------------------------------------------------------------ */

export function EmptyState({
  icon,
  title,
  hint,
  action,
}: {
  icon?: ReactNode;
  title: string;
  hint?: string;
  action?: ReactNode;
}) {
  return (
    <div className="flex flex-col items-center justify-center gap-3 rounded-[var(--radius-card)] border border-dashed border-edge bg-surface/50 px-6 py-14 text-center animate-fade-in">
      {icon && <div className="text-3xl opacity-70">{icon}</div>}
      <div>
        <p className="font-medium text-ink">{title}</p>
        {hint && <p className="mt-1 text-sm text-muted">{hint}</p>}
      </div>
      {action}
    </div>
  );
}

/* ------------------------------------------------------------------ */
/* Skeleton / Spinner / Kbd                                            */
/* ------------------------------------------------------------------ */

export function Skeleton({ className = "h-4 w-full" }: { className?: string }) {
  return <div className={`skeleton ${className}`} />;
}

export function Spinner({ className = "h-4 w-4" }: { className?: string }) {
  return (
    <span
      className={`inline-block rounded-full border-2 border-edge border-t-accent ${className}`}
      style={{ animation: "spin-fast 0.7s linear infinite" }}
    />
  );
}

export function Kbd({ children }: { children: ReactNode }) {
  return (
    <kbd className="rounded-md border border-edge bg-base px-1.5 py-0.5 font-mono text-[11px] text-muted">
      {children}
    </kbd>
  );
}

/* ------------------------------------------------------------------ */
/* Copy button                                                         */
/* ------------------------------------------------------------------ */

export function CopyButton({ text, label = "copy" }: { text: string; label?: string }) {
  const [copied, setCopied] = useState(false);
  const timer = useRef<ReturnType<typeof setTimeout>>(null);
  const copy = useCallback(() => {
    navigator.clipboard?.writeText(text).catch(() => {});
    setCopied(true);
    if (timer.current) clearTimeout(timer.current);
    timer.current = setTimeout(() => setCopied(false), 1400);
  }, [text]);
  return (
    <button
      onClick={(e) => {
        e.stopPropagation();
        copy();
      }}
      className={`font-mono text-[11px] transition-colors ${
        copied ? "text-ok" : "text-faint hover:text-accent"
      }`}
      title="copy to clipboard"
    >
      {copied ? "copied ✓" : label}
    </button>
  );
}

/* ------------------------------------------------------------------ */
/* Toasts                                                             */
/* ------------------------------------------------------------------ */

type Toast = { id: number; msg: string; tone: "ok" | "err" | "info" };
const ToasterCtx = createContext<(msg: string, tone?: Toast["tone"]) => void>(() => {});

export function useToast() {
  return useContext(ToasterCtx);
}

let toastSeq = 0;

export function ToasterProvider({ children }: { children: ReactNode }) {
  const [toasts, setToasts] = useState<Toast[]>([]);
  const push = useCallback((msg: string, tone: Toast["tone"] = "info") => {
    const id = ++toastSeq;
    setToasts((t) => [...t.slice(-3), { id, msg, tone }]);
    setTimeout(() => setToasts((t) => t.filter((x) => x.id !== id)), 3200);
  }, []);
  return (
    <ToasterCtx.Provider value={push}>
      {children}
      <div className="pointer-events-none fixed right-4 top-4 z-[100] flex w-72 flex-col gap-2">
        {toasts.map((t) => (
          <div
            key={t.id}
            className={`animate-slide-in-right rounded-lg border bg-surface px-3 py-2 text-sm shadow-xl ${
              t.tone === "ok"
                ? "border-accent/40 text-accent"
                : t.tone === "err"
                  ? "border-err/40 text-err"
                  : "border-edge text-ink"
            }`}
          >
            {t.msg}
          </div>
        ))}
      </div>
    </ToasterCtx.Provider>
  );
}
