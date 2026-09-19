import { useEffect, useState, type ReactNode } from "react";
import { Mail, Phone, RefreshCw, UserRound } from "lucide-react";
import {
  getAccountProfile,
  updateCurrentUserAvatar,
} from "@/api/account";
import { ContentCard } from "@/components/common/content-card";
import { FileUpload } from "@/components/common/file-upload";
import { PageHeader } from "@/components/common/page-header";
import { StatusTag } from "@/components/common/status-tag";
import { toast } from "@/components/common/toast-store";
import { Button } from "@/components/ui/button";
import { isApiError } from "@/lib/api-error";
import { formatDateTime } from "@/lib/datetime";
import { useAuthenticatedFileUrl } from "@/hooks/use-authenticated-file-url";
import { useAuthStore } from "@/store/auth-store";
import type { CurrentUser } from "@/types";

function InfoItem({
  label,
  value,
}: {
  label: string;
  value?: ReactNode;
}) {
  return (
    <div className="rounded-lg border border-border px-4 py-3">
      <div className="text-sm text-text-tertiary">{label}</div>
      <div className="mt-1 min-h-[22px] font-medium text-text-primary">
        {value || "-"}
      </div>
    </div>
  );
}

function getErrorMessage(error: unknown, fallback: string) {
  if (isApiError(error)) return error.message;
  if (error instanceof Error) return error.message;
  return fallback;
}

function syncCurrentUser(user: CurrentUser) {
  useAuthStore.setState({
    user,
    isAuthenticated: true,
    isLoadingUser: false,
  });
}

export function AccountProfilePage() {
  const user = useAuthStore((state) => state.user);
  const fetchCurrentUser = useAuthStore((state) => state.fetchCurrentUser);
  const isLoadingUser = useAuthStore((state) => state.isLoadingUser);
  const [refreshing, setRefreshing] = useState(false);
  const [updatingAvatar, setUpdatingAvatar] = useState(false);

  useEffect(() => {
    if (user || isLoadingUser) return;

    fetchCurrentUser().catch((error) => {
      toast.error({
        title: "当前用户信息加载失败",
        description: getErrorMessage(error, "请稍后重试"),
      });
    });
  }, [fetchCurrentUser, isLoadingUser, user]);

  const handleRefresh = async () => {
    setRefreshing(true);

    try {
      const nextUser = await getAccountProfile();
      syncCurrentUser(nextUser);
      toast.success("当前用户信息已刷新");
    } catch (error) {
      toast.error({
        title: "刷新失败",
        description: getErrorMessage(error, "请稍后重试"),
      });
    } finally {
      setRefreshing(false);
    }
  };

  const handleAvatarUploaded = async (accessUrl: string) => {
    setUpdatingAvatar(true);

    try {
      await updateCurrentUserAvatar({ avatar: accessUrl });
      const nextUser = await getAccountProfile();
      syncCurrentUser(nextUser);
      toast.success("头像已更新");
    } catch (error) {
      toast.error({
        title: "头像更新失败",
        description: getErrorMessage(error, "请稍后重试"),
      });
    } finally {
      setUpdatingAvatar(false);
    }
  };

  const rolesText =
    user?.roles?.map((role) => role.roleName).filter(Boolean).join("、") || "-";
  const roleCodesText =
    user?.roles?.map((role) => role.roleCode).filter(Boolean).join("、") || "-";
  const {
    url: avatarUrl,
    loading: avatarLoading,
  } = useAuthenticatedFileUrl(user?.avatar);

  return (
    <>
      <PageHeader
        title="个人中心"
        description="查看当前账号的基础资料、联系方式、角色和登录信息。"
        actions={
          <Button
            variant="secondary"
            disabled={refreshing}
            onClick={handleRefresh}
          >
            <RefreshCw
              className={refreshing ? "h-4 w-4 animate-spin" : "h-4 w-4"}
              aria-hidden
            />
            刷新
          </Button>
        }
      />

      <div className="grid max-w-[1120px] gap-6 xl:grid-cols-[320px_minmax(0,1fr)]">
        <ContentCard title="账号概览" bodyClassName="space-y-4">
          <div className="flex items-center gap-3">
            <div className="flex h-12 w-12 shrink-0 items-center justify-center overflow-hidden rounded-xl bg-blue-50 text-primary">
              {avatarUrl ? (
                <img
                  src={avatarUrl}
                  alt="当前用户头像"
                  className="h-full w-full object-cover"
                />
              ) : avatarLoading ? (
                <span className="text-xs text-text-tertiary">加载中</span>
              ) : (
                <UserRound className="h-6 w-6" aria-hidden />
              )}
            </div>
            <div className="min-w-0">
              <div className="truncate font-semibold text-text-primary">
                {user?.nickname || "加载中"}
              </div>
              <div className="truncate text-sm text-text-tertiary">
                {user?.username ?? "-"}
              </div>
            </div>
          </div>

          <div className="space-y-3 border-t border-border pt-4 text-sm">
            <div className="flex items-center justify-between gap-4">
              <span className="text-text-tertiary">账号状态</span>
              <StatusTag tone="success">当前账号</StatusTag>
            </div>
            <div className="flex items-center justify-between gap-4">
              <span className="text-text-tertiary">部门</span>
              <span className="truncate font-medium text-text-primary">
                {user?.dept?.deptName ?? "-"}
              </span>
            </div>
          </div>

          <div className="border-t border-border pt-4">
            <FileUpload
              accept="image/png,image/jpeg,image/gif"
              buttonText={updatingAvatar ? "更新中..." : "修改头像"}
              helperText="支持 PNG、JPEG、GIF；建议上传清晰的正方形图片。"
              businessModule="account-avatar"
              disabled={updatingAvatar}
              onAccessUrlChange={handleAvatarUploaded}
            />
          </div>
        </ContentCard>

        <div className="space-y-6">
          <ContentCard
            title="基础信息"
            description="当前登录账号的基础资料。"
          >
            <div className="grid gap-4 md:grid-cols-2">
              <InfoItem label="用户名" value={user?.username} />
              <InfoItem label="昵称" value={user?.nickname} />
              <InfoItem label="所属部门" value={user?.dept?.deptName} />
              <InfoItem label="最近登录时间" value={formatDateTime(user?.lastLoginTime)} />
              <InfoItem label="最近登录 IP" value={user?.lastLoginIp} />
              <InfoItem label="用户 ID" value={user?.id} />
            </div>
          </ContentCard>

          <ContentCard title="联系方式">
            <div className="grid gap-4 md:grid-cols-2">
              <InfoItem
                label="邮箱"
                value={
                  <span className="flex min-w-0 items-center gap-2">
                    <Mail className="h-4 w-4 shrink-0 text-text-tertiary" />
                    <span className="truncate">{user?.email || "-"}</span>
                  </span>
                }
              />
              <InfoItem
                label="手机号"
                value={
                  <span className="flex min-w-0 items-center gap-2">
                    <Phone className="h-4 w-4 shrink-0 text-text-tertiary" />
                    <span className="truncate">{user?.phone || "-"}</span>
                  </span>
                }
              />
            </div>
          </ContentCard>

          <ContentCard title="角色信息">
            <div className="grid gap-4 md:grid-cols-2">
              <InfoItem label="角色名称" value={rolesText} />
              <InfoItem label="角色编码" value={roleCodesText} />
            </div>
          </ContentCard>
        </div>
      </div>
    </>
  );
}
