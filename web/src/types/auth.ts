export type LoginRequest = {
  username: string;
  password: string;
};

export type LoginResponse = {
  tokenName: string;
  tokenValue: string;
  expiresIn: number;
};

export type CurrentUserDept = {
  id: number;
  deptName: string;
};

export type CurrentUserRole = {
  id: number;
  roleName: string;
  roleCode: string;
};

export type CurrentUser = {
  id: number;
  username: string;
  nickname: string;
  avatar?: string | null;
  phone?: string | null;
  email?: string | null;
  dept?: CurrentUserDept | null;
  roles: CurrentUserRole[];
  lastLoginTime?: string | null;
  lastLoginIp?: string | null;
};

export type CurrentUserMenu = {
  id: number;
  parentId: number;
  menuName: string;
  menuType: "DIR" | "MENU" | "LINK";
  path: string;
  component: string | null;
  externalUrl?: string | null;
  icon: string | null;
  permissionCode: string | null;
  sortOrder: number;
  visible: 0 | 1;
  children: CurrentUserMenu[];
};

export type AuthUser = {
  username: string;
  displayName: string;
};

export type AuthState = {
  token: string | null;
  user: CurrentUser | null;
  menus: CurrentUserMenu[];
  isAuthenticated: boolean;
  isLoadingUser: boolean;
  isLoadingMenus: boolean;
  login: (username: string, password: string) => Promise<CurrentUser>;
  fetchCurrentUser: () => Promise<CurrentUser | null>;
  fetchCurrentUserMenus: (force?: boolean) => Promise<CurrentUserMenu[]>;
  logout: () => Promise<void>;
  clearAuth: () => void;
};

export type LoginErrors = {
  username?: string;
  password?: string;
  account?: string;
};
