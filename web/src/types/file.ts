import type { ApiPageRequest, ApiStatus } from "./api";

export type FileRecord = {
  id: number;
  originalName: string;
  storageName: string;
  extension: string;
  mimeType: string;
  fileSize: number;
  accessUrl: string;
  businessModule?: string | null;
  remark?: string | null;
  status: ApiStatus;
  createTime: string;
};

export type FileUploadOptions = {
  businessModule?: string;
  remark?: string;
};

export type FileUploadFailedItem = {
  fileName: string;
  message: string;
  index: number;
};

export type FileUploadBatchResult = {
  succeeded: FileRecord[];
  failed: FileUploadFailedItem[];
};

export type FileListQuery = Partial<ApiPageRequest> & {
  originalName?: string;
  businessModule?: string;
  mimeType?: string;
  status?: ApiStatus | "all";
};

export type FileUpdateRequest = {
  businessModule?: string;
  remark?: string;
};

export type FileStatusRequest = {
  status: ApiStatus;
};

export type FileBatchDeleteRequest = {
  ids: number[];
};
