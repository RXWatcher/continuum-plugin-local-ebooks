import { useQuery } from "@tanstack/react-query";
import { diagnostics } from "@/lib/api";

export default function Diagnostics() {
  const q = useQuery({ queryKey: ["diagnostics"], queryFn: diagnostics });
  if (q.error) return <p className="text-sm text-destructive">{(q.error as Error).message}</p>;
  return (
    <pre className="rounded-md border bg-muted/30 p-3 text-xs">
      {JSON.stringify(q.data ?? {}, null, 2)}
    </pre>
  );
}
