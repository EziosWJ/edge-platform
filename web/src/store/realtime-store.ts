import { create } from "zustand";
import type { RealtimeCurrentValue, RealtimePoint } from "@/types/realtime";

export function realtimePointKey({ deviceId, pointKey }: RealtimePoint) {
  return `${deviceId}\u0000${pointKey}`;
}

export function mergeRealtimeValue(
  current: RealtimeCurrentValue | undefined,
  incoming: RealtimeCurrentValue,
) {
  if (!current || incoming.revision > current.revision) return incoming;
  return current;
}

type RealtimeStore = {
  values: Record<string, RealtimeCurrentValue>;
  applyValue: (incoming: RealtimeCurrentValue) => boolean;
  clear: () => void;
};

export const useRealtimeStore = create<RealtimeStore>((set, get) => ({
  values: {},
  applyValue: (incoming) => {
    const key = realtimePointKey(incoming);
    const current = get().values[key];
    const merged = mergeRealtimeValue(current, incoming);
    if (merged === current) return false;

    set((state) => ({ values: { ...state.values, [key]: merged } }));
    return true;
  },
  clear: () => set({ values: {} }),
}));
