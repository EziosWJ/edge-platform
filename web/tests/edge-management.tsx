import { createRoot } from "react-dom/client";
import { EdgePage } from "@/pages/edge";
import "@/styles/globals.css";

declare global {
  interface Window {
    edgeRequests: string[];
  }
}

const records = [
  {
    edgeId: "edge-offline",
    status: "OFFLINE",
    registeredAt: "2026-09-21T08:00:00Z",
    lastSeenAt: "2026-09-21T08:30:00Z",
  },
  {
    edgeId: "edge-online",
    status: "ONLINE",
    registeredAt: "2026-09-20T08:00:00Z",
    lastSeenAt: "2026-09-21T08:31:00Z",
  },
];

window.edgeRequests = [];
window.fetch = async (input) => {
  const url = new URL(String(input), window.location.origin);
  window.edgeRequests.push(`${url.pathname}${url.search}`);

  if (url.pathname === "/api/edge/page") {
    const page = Number(url.searchParams.get("page") ?? "1");
    const pageSize = Number(url.searchParams.get("pageSize") ?? "10");
    const status = url.searchParams.get("status");
    const edgeId = url.searchParams.get("edgeId");
    const filtered = records.filter(
      (record) =>
        (!status || record.status === status) &&
        (!edgeId || record.edgeId === edgeId),
    );

    return new Response(
      JSON.stringify({
        code: 200,
        message: "success",
        data: {
          records: filtered,
          total: 21,
          page,
          pageSize,
        },
      }),
      { headers: { "Content-Type": "application/json" } },
    );
  }

  if (url.pathname === "/api/edge/edge-offline") {
    return new Response(
      JSON.stringify({ code: 200, message: "success", data: records[0] }),
      { headers: { "Content-Type": "application/json" } },
    );
  }

  return new Response(
    JSON.stringify({ code: 404, message: "Edge not found", data: null }),
    { status: 404, headers: { "Content-Type": "application/json" } },
  );
};

createRoot(document.getElementById("root")!).render(<EdgePage />);
