import { Link } from "@tanstack/react-router";
import { cloneElement, useEffect, useId, useState, type ReactElement, type ReactNode } from "react";
import { useQuery } from "@tanstack/react-query";
import { useSpatial } from "./context";
import { inputClass } from "./state";
export function Field({
  label,
  children,
}: {
  label: string;
  children: ReactElement<{ id?: string }>;
}) {
  const generatedID = useId(), id = children.props.id ?? generatedID;
  return (
    <div className="grid gap-1.5 text-sm">
      <label htmlFor={id} className="font-medium">{label}</label>
      {cloneElement(children, { id })}
    </div>
  );
}
export function TextField({
  label,
  value,
  onChange,
  type = "text",
  disabled = false,
}: {
  label: string;
  value: string;
  onChange: (v: string) => void;
  type?: string;
  disabled?: boolean;
}) {
  return (
    <Field label={label}>
      <input
        className={inputClass}
        type={type}
        value={value}
        onChange={(e) => onChange(e.target.value)}
        disabled={disabled}
      />
    </Field>
  );
}
export function Failure({ error }: { error: unknown }) {
  return error ? (
    <p
      role="alert"
      className="break-words rounded-md border border-destructive/40 bg-destructive/5 p-3 text-sm text-destructive"
    >
      {error instanceof Error ? error.message : String(error)}
    </p>
  ) : null;
}
export function Info({ children }: { children: ReactNode }) {
  return (
    <p
      role="status"
      className="rounded-md bg-muted/50 p-3 text-sm text-muted-foreground"
    >
      {children}
    </p>
  );
}
// eslint-disable-next-line react-refresh/only-export-components
export function useSpatialFile(version: string, file?: string) {
  const m = useSpatial();
  return useQuery({
    queryKey: [...m.prefix, "file", version, file],
    queryFn: ({ signal }) =>
      m.client.spatialFile(m.scope, version, file!, signal),
    enabled: m.available && !!file,
    retry: false,
    staleTime: Infinity,
    gcTime: 0,
  });
}
// eslint-disable-next-line react-refresh/only-export-components
export function useBlobURL(blob?: Blob) {
  const [value, setValue] = useState<{ blob: Blob; url: string }>();
  useEffect(() => {
    if (!blob) return;
    const url = URL.createObjectURL(blob);
    // The browser-owned object URL must be created and released with the effect lifecycle.
    // eslint-disable-next-line react-hooks/set-state-in-effect
    setValue({ blob, url });
    return () => URL.revokeObjectURL(url);
  }, [blob]);
  return value?.blob === blob ? value?.url : undefined;
}

export function SpatialLink({
  to,
  search,
  children,
  className,
}: {
  to: string;
  search?: Record<string, unknown>;
  children: ReactNode;
  className?: string;
}) {
  return (
    <Link to={to} search={search} className={className}>
      {children}
    </Link>
  );
}
