import { buildApiUrl, http } from "@/lib/http";
import type { ApiPageResult } from "@/types/api";
import type {
  FileBatchDeleteRequest,
  FileListQuery,
  FileRecord,
  FileStatusRequest,
  FileUpdateRequest,
  FileUploadBatchResult,
  FileUploadOptions,
} from "@/types/file";

const FILE_BASE_PATH = "/api/system/file";

function appendOptionalText(
  formData: FormData,
  key: string,
  value: string | undefined,
) {
  if (value === undefined || value === "") return;
  formData.append(key, value);
}

function createUploadFormData(
  key: "file" | "files",
  files: File | File[],
  options: FileUploadOptions = {},
) {
  const formData = new FormData();
  const fileItems = Array.isArray(files) ? files : [files];

  fileItems.forEach((file) => formData.append(key, file));
  appendOptionalText(formData, "businessModule", options.businessModule);
  appendOptionalText(formData, "remark", options.remark);

  return formData;
}

export function uploadFile(file: File, options?: FileUploadOptions) {
  return http.post<FileRecord>(
    `${FILE_BASE_PATH}/upload`,
    createUploadFormData("file", file, options),
  );
}

export function uploadFiles(files: File[], options?: FileUploadOptions) {
  return http.post<FileUploadBatchResult>(
    `${FILE_BASE_PATH}/upload-batch`,
    createUploadFormData("files", files, options),
  );
}

export function getFilePage(query: FileListQuery) {
  return http.get<ApiPageResult<FileRecord>>(`${FILE_BASE_PATH}/page`, {
    query,
  });
}

export function getFileDetail(id: number) {
  return http.get<FileRecord>(`${FILE_BASE_PATH}/${id}`);
}

export function updateFile(id: number, data: FileUpdateRequest) {
  return http.put<void>(`${FILE_BASE_PATH}/${id}`, data);
}

export function deleteFile(id: number) {
  return http.delete<void>(`${FILE_BASE_PATH}/${id}`);
}

export function batchDeleteFiles(data: FileBatchDeleteRequest) {
  return http.post<void>(`${FILE_BASE_PATH}/batch-delete`, data);
}

export function updateFileStatus(id: number, data: FileStatusRequest) {
  return http.patch<void>(`${FILE_BASE_PATH}/${id}/status`, data);
}

export function downloadFile(id: number) {
  return http.blob(`${FILE_BASE_PATH}/${id}/download`);
}

export function getFileViewUrl(id: number) {
  return buildApiUrl(`${FILE_BASE_PATH}/${id}/view`);
}

export function getFileAccessUrl(accessUrl: string) {
  return buildApiUrl(accessUrl);
}
