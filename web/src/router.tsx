import { createBrowserRouter, Navigate } from "react-router-dom";
import { RequireAuth } from "@/components/auth/require-auth";
import { AppShell } from "@/components/layout/app-shell";
import { AccountProfilePage } from "@/pages/account-profile";
import { ChangePasswordPage } from "@/pages/change-password";
import { DashboardPage } from "@/pages/dashboard";
import { LoginPage } from "@/pages/login";
import { NotFoundPage } from "@/pages/not-found";
import { SettingsPage } from "@/pages/settings";
import { SystemConfigsPage } from "@/pages/system/configs";
import { SystemDeptsPage } from "@/pages/system/depts";
import { SystemDictsPage } from "@/pages/system/dicts";
import { SystemFilesPage } from "@/pages/system/files";
import { SystemLoginLogsPage } from "@/pages/system/logs/login-logs";
import { SystemOperLogsPage } from "@/pages/system/logs/oper-logs";
import { SystemMenusPage } from "@/pages/system/menus";
import { SystemRolesPage } from "@/pages/system/roles";
import { UsersPage } from "@/pages/system/users";
import { NotificationsPage } from "@/pages/notifications";
import { NotificationManagePage } from "@/pages/system/notifications";
import { EdgePage } from "@/pages/edge";
import { DevicePage } from "@/pages/device";

export const router = createBrowserRouter([
  { path: "/login", element: <LoginPage /> },
  {
    path: "/",
    element: (
      <RequireAuth>
        <AppShell />
      </RequireAuth>
    ),
    children: [
      { index: true, element: <Navigate to="/dashboard" replace /> },
      { path: "dashboard", element: <DashboardPage /> },
      { path: "edge", element: <EdgePage /> },
      { path: "device", element: <DevicePage /> },
      { path: "settings", element: <SettingsPage /> },
      { path: "system", element: <Navigate to="/system/user" replace /> },
      { path: "system/user", element: <UsersPage /> },
      { path: "system/dept", element: <SystemDeptsPage /> },
      { path: "system/dict", element: <SystemDictsPage /> },
      { path: "system/config", element: <SystemConfigsPage /> },
      { path: "system/role", element: <SystemRolesPage /> },
      { path: "system/menu", element: <SystemMenusPage /> },
      { path: "system/login-log", element: <SystemLoginLogsPage /> },
      { path: "system/oper-log", element: <SystemOperLogsPage /> },
      { path: "system/file", element: <SystemFilesPage /> },
      { path: "notifications", element: <NotificationsPage /> },
      { path: "notifications/:id", element: <NotificationsPage /> },
      { path: "system/notification", element: <NotificationManagePage /> },
      { path: "account/profile", element: <AccountProfilePage /> },
      { path: "account/change-password", element: <ChangePasswordPage /> },
      { path: "*", element: <NotFoundPage /> },
    ],
  },
]);
