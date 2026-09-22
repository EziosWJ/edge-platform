import type { ApiPageResult } from "@/types/api";
import type {
	CommandPageQuery,
	CommandRecord,
} from "@/types/command";
import { http } from "@/lib/http";

const COMMAND_BASE_PATH = "/api/command";

export function getCommandPage(query: CommandPageQuery) {
  return http.get<ApiPageResult<CommandRecord>>(`${COMMAND_BASE_PATH}/page`, {
    query,
  });
}

export function getCommandDetail(commandId: string, signal?: AbortSignal) {
  return http.get<CommandRecord>(
    `${COMMAND_BASE_PATH}/${encodeURIComponent(commandId)}`,
    { signal },
  );
}

export type CreateCommandInput = {
  commandId: string;
  deviceId: string;
  name: string;
  argsJson: string;
  ttlSeconds: number;
};

export function createCommand(body: CreateCommandInput) {
  // Keep the user's JSON number tokens intact when assembling the request;
  // JSON.parse/JSON.stringify would round large values through JavaScript.
  const raw = `{"commandId":${JSON.stringify(body.commandId)},"deviceId":${JSON.stringify(body.deviceId)},"name":${JSON.stringify(body.name)},"args":${body.argsJson},"ttlSeconds":${body.ttlSeconds}}`;
  return http.post<CommandRecord>(COMMAND_BASE_PATH, raw);
}
