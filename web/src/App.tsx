import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import Libraries from "./pages/Libraries";
import Scans from "./pages/Scans";
import Metadata from "./pages/Metadata";
import Diagnostics from "./pages/Diagnostics";

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
        <TabsContent value="scans"><Scans /></TabsContent>
        <TabsContent value="metadata"><Metadata /></TabsContent>
        <TabsContent value="diagnostics"><Diagnostics /></TabsContent>
      </Tabs>
    </div>
  );
}
