import { useEffect, useRef, useState } from "react";
import { RealtimeClient } from "@/realtime/client";
import {
  realtimePointKey,
  useRealtimeStore,
} from "@/store/realtime-store";
import type {
  RealtimeConnectionStatus,
  RealtimeCurrentValue,
} from "@/types/realtime";

type UseRealtimePointOptions = {
  deviceId: string;
  pointKey: string;
  enabled?: boolean;
};

export function useRealtimePoint({
  deviceId,
  pointKey,
  enabled = true,
}: UseRealtimePointOptions) {
  const value = useRealtimeValue({ deviceId, pointKey });
  const result = useRealtimePoints({ points: [{ deviceId, pointKey }], enabled });
  return { value, status: result.status, error: result.error };
}

export function useRealtimeValue({ deviceId, pointKey }: { deviceId: string; pointKey: string }) {
  return useRealtimeStore((state) => {
    if (!deviceId || !pointKey) return undefined;
    return state.values[realtimePointKey({ deviceId, pointKey })];
  });
}

export function useRealtimePoints({
  points,
  enabled = true,
}: {
  points: Array<{ deviceId: string; pointKey: string }>;
  enabled?: boolean;
}) {
  const [status, setStatus] = useState<RealtimeConnectionStatus>("idle");
  const [error, setError] = useState<Error | null>(null);
  const pointsRef = useRef(points);
  pointsRef.current = points;
  const clientRef = useRef<RealtimeClient | null>(null);
  const pointSignature = points.map((point) => `${point.deviceId}\u0000${point.pointKey}`).join("\u0001");

  useEffect(() => {
    const validPoints = pointsRef.current.filter((point) => point.deviceId && point.pointKey);
    if (!enabled) {
      clientRef.current?.disconnect();
      clientRef.current = null;
      setStatus("idle");
      setError(null);
      return;
    }

    let active = true;
    const client = new RealtimeClient({
      points: validPoints,
      onValue: (incoming: RealtimeCurrentValue) => {
        useRealtimeStore.getState().applyValue(incoming);
      },
      onStatus: (nextStatus) => {
        if (active) setStatus(nextStatus);
      },
      onError: (nextError) => {
        if (active) setError(nextError);
      },
    });
    clientRef.current = client;

    setError(null);
    void client.connect().catch(() => undefined);

    return () => {
      active = false;
      if (clientRef.current === client) {
        client.disconnect();
        clientRef.current = null;
      }
    };
  }, [enabled]);

  useEffect(() => {
    const client = clientRef.current;
    if (!client) return;
    const validPoints = pointsRef.current.filter((point) => point.deviceId && point.pointKey);
    client.setPoints(validPoints);
    void client.connect().catch(() => undefined);
  }, [pointSignature]);

  return { status, error };
}
