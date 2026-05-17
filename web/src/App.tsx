import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import Libraries from "./pages/Libraries";

export default function App() {
  return (
    <div className="mx-auto max-w-6xl space-y-4 p-6">
      <h1 className="text-2xl font-semibold">Local Ebooks — Operator Console</h1>
      <Tabs defaultValue="libraries">
        <TabsList>
          <TabsTrigger value="libraries">Libraries</TabsTrigger>
          <TabsTrigger value="scans">Scans</TabsTrigger>
          <TabsTrigger value="metadata">Metadata</TabsTrigger>
          <TabsTrigger value="diagnostics">Diagnostics</TabsTrigger>
        </TabsList>
        <TabsContent value="libraries"><Libraries /></TabsContent>
        <TabsContent value="scans"><div className="text-sm text-muted-foreground">See Task 12.</div></TabsContent>
        <TabsContent value="metadata"><div className="text-sm text-muted-foreground">See Task 12.</div></TabsContent>
        <TabsContent value="diagnostics"><div className="text-sm text-muted-foreground">See Task 12.</div></TabsContent>
      </Tabs>
    </div>
  );
}
