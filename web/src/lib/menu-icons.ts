import {
  BarChart3,
  Bell,
  Boxes,
  Building2,
  CircleDot,
  Database,
  FileText,
  GitBranch,
  Home,
  KeyRound,
  Link,
  ListTree,
  LockKeyhole,
  Menu,
  MonitorCog,
  Package,
  ScrollText,
  Settings,
  ShieldHalf,
  SlidersHorizontal,
  User,
  Users,
  type LucideIcon,
} from "lucide-react";

const MENU_ICON_MAP: Record<string, LucideIcon> = {
  dashboard: Home,
  home: Home,
  system: Settings,
  setting: Settings,
  settings: Settings,
  user: User,
  users: Users,
  role: ShieldHalf,
  roles: ShieldHalf,
  shield: ShieldHalf,
  menu: Menu,
  menus: Menu,
  tree: ListTree,
  "list-tree": ListTree,
  dept: Building2,
  department: Building2,
  dict: ListTree,
  dictionary: ListTree,
  config: SlidersHorizontal,
  configs: SlidersHorizontal,
  log: ScrollText,
  logs: ScrollText,
  file: FileText,
  files: FileText,
  database: Database,
  permission: KeyRound,
  permissions: KeyRound,
  lock: LockKeyhole,
  link: Link,
  notice: Bell,
  chart: BarChart3,
  package: Package,
  module: Boxes,
  branch: GitBranch,
  monitor: MonitorCog,
};

function normalizeIconName(icon?: string | null) {
  return icon?.trim().replace(/([a-z0-9])([A-Z])/g, "$1-$2").toLowerCase();
}

export function getMenuIcon(icon?: string | null): LucideIcon {
  const normalized = normalizeIconName(icon);
  if (!normalized) return CircleDot;
  return MENU_ICON_MAP[normalized] ?? CircleDot;
}
