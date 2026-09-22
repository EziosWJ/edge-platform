import { http } from "@/lib/http";
import type { RealtimeTicketResponse } from "@/types/realtime";

export function createRealtimeTicket() {
  return http.post<RealtimeTicketResponse>("/api/realtime/ticket");
}
