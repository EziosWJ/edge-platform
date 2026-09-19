export type AccountProfile = {
  username: string;
  displayName: string;
  email: string;
  phone: string;
  department: string;
  role: string;
  roleCode: string;
  lastLogin: string;
  status: "enabled" | "disabled";
};

export type ChangePasswordRequest = {
  oldPassword: string;
  newPassword: string;
};

export type UpdateAvatarRequest = {
  avatar: string;
};
