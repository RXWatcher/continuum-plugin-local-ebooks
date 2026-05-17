import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { listScans, scanAll } from "@/lib/api";
import { Button } from "@/components/ui/button";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";

export default function Scans() {
  const qc = useQueryClient();
  const q = useQuery({ queryKey: ["scans"], queryFn: listScans });
  const all = useMutation({
    mutationFn: scanAll,
    onSuccess: () => { toast.success("Scan started"); qc.invalidateQueries({ queryKey: ["scans"] }); },
    onError: (e: Error) => toast.error(e.message),
  });
  if (q.error) return <p className="text-sm text-destructive">{(q.error as Error).message}</p>;
  return (
    <div className="space-y-3">
      <Button onClick={() => all.mutate()} disabled={all.isPending}>Scan all libraries</Button>
      <Table>
        <TableHeader><TableRow>
          <TableHead>Library</TableHead><TableHead>Started</TableHead><TableHead>Finished</TableHead>
          <TableHead>+/~/-</TableHead><TableHead>Error</TableHead>
        </TableRow></TableHeader>
        <TableBody>
          {(q.data?.items ?? []).map((s) => (
            <TableRow key={s.id}>
              <TableCell>{s.library_name || "all"}</TableCell>
              <TableCell className="text-xs">{s.started_at}</TableCell>
              <TableCell className="text-xs">{s.finished_at ?? "running"}</TableCell>
              <TableCell>{s.books_added}/{s.books_changed}/{s.books_deleted}</TableCell>
              <TableCell className="text-xs text-destructive">{s.error_text}</TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </div>
  );
}
