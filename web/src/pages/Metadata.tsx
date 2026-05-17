import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { metadataBackfill, metadataQueue } from "@/lib/api";
import { Button } from "@/components/ui/button";

export default function Metadata() {
  const qc = useQueryClient();
  const q = useQuery({ queryKey: ["meta-queue"], queryFn: metadataQueue });
  const backfill = useMutation({
    mutationFn: metadataBackfill,
    onSuccess: (r) => { toast.success(`Queued ${r.queued}`); qc.invalidateQueries({ queryKey: ["meta-queue"] }); },
    onError: (e: Error) => toast.error(e.message),
  });
  if (q.error) return <p className="text-sm text-destructive">{(q.error as Error).message}</p>;
  return (
    <div className="space-y-3">
      <pre className="rounded-md border bg-muted/30 p-3 text-xs">{JSON.stringify(q.data ?? {}, null, 2)}</pre>
      <Button onClick={() => backfill.mutate()} disabled={backfill.isPending}>Backfill all</Button>
    </div>
  );
}
