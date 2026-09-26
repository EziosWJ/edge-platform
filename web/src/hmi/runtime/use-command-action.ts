import { useCallback, useEffect, useRef, useState } from "react";
import { createCommand, getCommandDetail } from "@/api/command";
import type { CommandBinding } from "@/hmi/model";
import type { CommandRecord, CommandStatus } from "@/types/command";

export type HmiCommandFeedback = {
  status: "idle" | "submitting" | "pending" | "accepted" | "succeeded" | "failed";
  message?: string;
};

const POLL_INTERVAL_MS = 2_000;
const TERMINAL_STATUSES = new Set<CommandStatus>([
  "REJECTED",
  "EXPIRED",
  "SUCCEEDED",
  "FAILED",
]);

function feedbackFor(command: CommandRecord): HmiCommandFeedback {
  switch (command.status) {
    case "SUCCEEDED":
      return { status: "succeeded", message: "命令执行成功" };
    case "REJECTED":
      return { status: "failed", message: command.errorMessage || "命令已拒绝" };
    case "EXPIRED":
      return { status: "failed", message: "命令已过期" };
    case "FAILED":
      return { status: "failed", message: command.errorMessage || "命令执行失败" };
    case "ACCEPTED":
      return { status: "accepted", message: "边缘设备已接收，等待执行结果" };
    default:
      return { status: "pending", message: "命令已提交，等待设备响应" };
  }
}

export function useHmiCommandAction() {
  const [feedback, setFeedback] = useState<HmiCommandFeedback>({ status: "idle" });
  const [busy, setBusy] = useState(false);
  const busyRef = useRef(false);
  const mounted = useRef(true);
  const timer = useRef<number | undefined>(undefined);
  const controller = useRef<AbortController | null>(null);

  useEffect(() => {
    mounted.current = true;
    return () => {
      mounted.current = false;
      if (timer.current !== undefined) window.clearTimeout(timer.current);
      controller.current?.abort();
    };
  }, []);

  const poll = useCallback(async (commandId: string) => {
    if (!mounted.current) return;
    controller.current?.abort();
    const request = new AbortController();
    controller.current = request;
    try {
      const command = await getCommandDetail(commandId, request.signal);
      if (!mounted.current) return;
      setFeedback(feedbackFor(command));
      if (TERMINAL_STATUSES.has(command.status)) {
        busyRef.current = false;
        setBusy(false);
        return;
      }
    } catch (error) {
      if (!mounted.current || request.signal.aborted) return;
      setFeedback({
        status: "pending",
        message: error instanceof Error
          ? `命令状态暂时不可用：${error.message}`
          : "命令状态暂时不可用，正在重试",
      });
    }
    if (mounted.current) {
      timer.current = window.setTimeout(() => void poll(commandId), POLL_INTERVAL_MS);
    }
  }, []);

  const run = useCallback(async (binding: CommandBinding) => {
    if (busyRef.current) return;
    if (binding.confirmation.required && !window.confirm(binding.confirmation.message || "确认执行此操作？")) return;

    busyRef.current = true;
    setBusy(true);
    setFeedback({ status: "submitting", message: "正在提交命令" });
    const commandId = crypto.randomUUID();
    try {
      const command = await createCommand({
        commandId,
        deviceId: binding.deviceId,
        name: binding.name,
        argsJson: JSON.stringify(binding.args),
        ttlSeconds: binding.ttlSeconds,
      });
      if (!mounted.current) return;
      setFeedback(feedbackFor(command));
      if (TERMINAL_STATUSES.has(command.status)) {
        busyRef.current = false;
        setBusy(false);
        return;
      }
      timer.current = window.setTimeout(() => void poll(commandId), POLL_INTERVAL_MS);
    } catch (error) {
      if (!mounted.current) return;
      busyRef.current = false;
      setBusy(false);
      setFeedback({
        status: "failed",
        message: error instanceof Error ? error.message : "命令提交失败",
      });
    }
  }, [poll]);

  return { run, busy, feedback };
}
