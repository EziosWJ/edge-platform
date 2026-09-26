import { HmiComponentRenderer, type HmiCommandFeedback } from "@/hmi/registry";
import type { HmiNode, DataPointBinding } from "@/hmi/model";
import { realtimePointKey } from "@/store/realtime-store";
import { useRealtimeValue } from "@/hooks/use-realtime-point";
import type { HmiRuntimeDataPoint } from "@/hmi/runtime/api";
import { useHmiCommandAction } from "@/hmi/runtime/use-command-action";

type RuntimeNodeProps = {
  node: HmiNode;
  dataPoints: Record<string, HmiRuntimeDataPoint>;
  canExecuteCommands: boolean;
};

function getDataPointBinding(node: HmiNode): DataPointBinding | undefined {
  if (node.type === "switch") {
    const binding = node.bindings.state;
    return binding?.kind === "datapoint" ? binding : undefined;
  }
  const binding = node.bindings.value;
  return binding?.kind === "datapoint" ? binding : undefined;
}

export function HmiRuntimeNode({
  node,
  dataPoints,
  canExecuteCommands,
}: RuntimeNodeProps) {
  const commandAction = useHmiCommandAction();
  const pointBinding = getDataPointBinding(node);
  const point = pointBinding ?? { deviceId: "", pointKey: "" };
  const value = useRealtimeValue(point);
  const metadata = pointBinding
    ? dataPoints[realtimePointKey(pointBinding)]
    : undefined;
  const renderedValue = metadata?.enabled ? value : undefined;
  const commandBinding = node.type === "button"
    ? node.bindings.command
    : undefined;
  const actionable = commandBinding?.kind === "command" || node.type === "switch";
  const onControlCommand = actionable && canExecuteCommands
    ? commandAction.run
    : undefined;
  const feedback: HmiCommandFeedback | undefined = actionable
    ? canExecuteCommands
      ? commandAction.feedback
      : { status: "failed", message: "当前账号没有命令执行权限" }
    : undefined;

  return (
    <HmiComponentRenderer
      node={node}
      mode="runtime"
      value={renderedValue}
      metadata={metadata}
      onCommand={onControlCommand}
      commandState={feedback}
    />
  );
}
