import type { ReactNode } from "react";

export type DataTableColumn<T> = {
  title: ReactNode;
  dataIndex?: keyof T;
  key?: string;
  width?: number | string;
  align?: "left" | "center" | "right";
  nowrap?: boolean;
  render?: (value: T[keyof T] | undefined, record: T, index: number) => ReactNode;
};
