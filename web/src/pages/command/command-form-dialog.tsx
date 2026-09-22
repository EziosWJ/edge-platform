import { useEffect, useState } from "react";
import { createCommand } from "@/api/command";
import { FormDialog } from "@/components/common/form-dialog";
import { Field } from "@/components/common/field";
import { toast } from "@/components/common/toast-store";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { getErrorMessage } from "@/lib/api-error";
import type { CommandRecord, DeviceCommunicationStatus } from "@/types";

type CommandFormDialogProps = {
  open: boolean;
  deviceId: string | null;
  communicationStatus?: DeviceCommunicationStatus;
  onCancel: () => void;
  onCreated: (command: CommandRecord) => void;
};

function newCommandId() {
  return crypto.randomUUID();
}

export function CommandFormDialog({
  open,
  deviceId,
  communicationStatus,
  onCancel,
  onCreated,
}: CommandFormDialogProps) {
  const [name, setName] = useState("");
  const [args, setArgs] = useState("{}");
  const [ttl, setTtl] = useState("30");
  const [loading, setLoading] = useState(false);
  const [argsError, setArgsError] = useState("");

  useEffect(() => {
    if (!open) return;
    setName("");
    setArgs("{}");
    setTtl("30");
    setArgsError("");
  }, [open, deviceId]);

  const submit = async () => {
    const trimmedName = name.trim();
    const ttlSeconds = Number(ttl);
    if (!trimmedName) {
      toast.error("请输入 Command name");
      return;
    }
    if (!Number.isInteger(ttlSeconds) || ttlSeconds < 1 || ttlSeconds > 300) {
      toast.error("TTL 必须是 1-300 秒的整数");
      return;
    }
    try {
      const parsed = JSON.parse(args) as unknown;
      if (parsed === null || Array.isArray(parsed) || typeof parsed !== "object") {
        throw new Error("args 必须是 JSON object");
      }
    } catch (error) {
      setArgsError(error instanceof Error ? error.message : "args 必须是合法 JSON object");
      return;
    }
    if (!deviceId) return;

    setLoading(true);
    try {
      const created = await createCommand({
        commandId: newCommandId(),
        deviceId,
        name: trimmedName,
        argsJson: args,
        ttlSeconds,
      });
      toast.success("Command 已提交，当前等待执行");
      onCreated(created);
    } catch (error) {
      toast.error({ title: "Command 提交失败", description: getErrorMessage(error, "请稍后重试") });
    } finally {
      setLoading(false);
    }
  };

  const riskMessage = communicationStatus === "OFFLINE"
    ? "Device 最近一次状态为离线，提交仍会按 Cloud TTL 受理。"
    : communicationStatus === "DEGRADED"
      ? "Device 最近一次状态为降级，提交仍会按 Cloud TTL 受理。"
      : "状态提示仅供参考，不会阻止 Cloud 受理。";

  return (
    <FormDialog
      open={open}
      title="执行 Command"
      description={`Cloud Device ID：${deviceId ?? "-"}`}
      loading={loading}
      submitText="提交执行"
      loadingText="正在受理…"
      onCancel={onCancel}
      onSubmit={submit}
    >
      <div className="space-y-space-4">
        <div className="border-l-2 border-warning bg-warning-background px-space-3 py-space-2 text-sm text-warning" role="note">
          {riskMessage}
        </div>
        <Field label="Command name" required help="输入当前 Device 支持的控制名称；Cloud 不提供 schema discovery。">
          <Input value={name} onChange={(event) => setName(event.target.value)} autoFocus placeholder="例如 close" />
        </Field>
        <Field label="JSON args" required error={argsError} help={'必须是 JSON object，例如 {"mode":"safe"}。'}>
          <Textarea value={args} onChange={(event) => { setArgs(event.target.value); setArgsError(""); }} rows={7} spellCheck={false} aria-label="JSON args" />
        </Field>
        <Field label="TTL（秒）" required help="Cloud 受理有效期，范围 1-300 秒，默认 30 秒。">
          <Input type="number" min={1} max={300} step={1} value={ttl} onChange={(event) => setTtl(event.target.value)} />
        </Field>
      </div>
    </FormDialog>
  );
}
