import { http } from "@/lib/http";
import type { ApiPageResult } from "@/types/api";
import type {
  DictDataBatchDeleteRequest,
  DictDataCreateRequest,
  DictDataListQuery,
  DictDataUpdateRequest,
  DictOption,
  DictTypeBatchDeleteRequest,
  DictTypeCreateRequest,
  DictTypeListQuery,
  DictTypeStatusRequest,
  DictTypeUpdateRequest,
  SystemConfigBatchDeleteRequest,
  SystemConfigCreateRequest,
  SystemConfigListQuery,
  SystemDictDataRecord,
  SystemDictTypeRecord,
  SystemConfigRecord,
  SystemConfigStatusRequest,
  SystemConfigUpdateRequest,
  SystemConfigValueRecord,
} from "@/types";

export async function getDictTypes(
  query: DictTypeListQuery,
): Promise<ApiPageResult<SystemDictTypeRecord>> {
  return getDictTypePage(query);
}

const DICT_TYPE_BASE_PATH = "/api/system/dict-type";
const DICT_DATA_BASE_PATH = "/api/system/dict-data";
const SYSTEM_CONFIG_BASE_PATH = "/api/system/config";
const dictItemCache = new Map<string, Promise<DictOption[]>>();
const systemConfigValueCache = new Map<string, Promise<SystemConfigValueRecord>>();

export function getDictTypePage(query: DictTypeListQuery) {
  return http.get<ApiPageResult<SystemDictTypeRecord>>(
    `${DICT_TYPE_BASE_PATH}/page`,
    {
      query,
    },
  );
}

export function getDictTypeDetail(id: number) {
  return http.get<SystemDictTypeRecord>(`${DICT_TYPE_BASE_PATH}/${id}`);
}

export function createDictType(data: DictTypeCreateRequest) {
  return http.post<void>(DICT_TYPE_BASE_PATH, data);
}

export function updateDictType(id: number, data: DictTypeUpdateRequest) {
  return http.put<void>(`${DICT_TYPE_BASE_PATH}/${id}`, data);
}

export function deleteDictType(id: number) {
  return http.delete<void>(`${DICT_TYPE_BASE_PATH}/${id}`);
}

export function batchDeleteDictTypes(data: DictTypeBatchDeleteRequest) {
  return http.post<void>(`${DICT_TYPE_BASE_PATH}/batch-delete`, data);
}

export function updateDictTypeStatus(id: number, data: DictTypeStatusRequest) {
  return http.patch<void>(`${DICT_TYPE_BASE_PATH}/${id}/status`, data);
}

export function getDictDataPage(query: DictDataListQuery) {
  return http.get<ApiPageResult<SystemDictDataRecord>>(
    `${DICT_DATA_BASE_PATH}/page`,
    {
      query,
    },
  );
}

export function getDictDataDetail(id: number) {
  return http.get<SystemDictDataRecord>(`${DICT_DATA_BASE_PATH}/${id}`);
}

export function createDictData(data: DictDataCreateRequest) {
  clearDictItemCache();
  return http.post<void>(DICT_DATA_BASE_PATH, data);
}

export function updateDictData(id: number, data: DictDataUpdateRequest) {
  clearDictItemCache();
  return http.put<void>(`${DICT_DATA_BASE_PATH}/${id}`, data);
}

export function deleteDictData(id: number) {
  clearDictItemCache();
  return http.delete<void>(`${DICT_DATA_BASE_PATH}/${id}`);
}

export function batchDeleteDictData(data: DictDataBatchDeleteRequest) {
  clearDictItemCache();
  return http.post<void>(`${DICT_DATA_BASE_PATH}/batch-delete`, data);
}

export function getDictItems(dictCode: string, forceRefresh = false) {
  const normalizedCode = dictCode.trim();

  if (!normalizedCode) {
    return Promise.resolve([]);
  }

  if (forceRefresh || !dictItemCache.has(normalizedCode)) {
    dictItemCache.set(
      normalizedCode,
      http.get<DictOption[]>(
        `/api/system/dict/${encodeURIComponent(normalizedCode)}/items`,
      ),
    );
  }

  return dictItemCache.get(normalizedCode) as Promise<DictOption[]>;
}

export function clearDictItemCache(dictCode?: string) {
  if (dictCode) {
    dictItemCache.delete(dictCode);
    return;
  }

  dictItemCache.clear();
}

export function getSystemConfigPage(query: SystemConfigListQuery) {
  return http.get<ApiPageResult<SystemConfigRecord>>(
    `${SYSTEM_CONFIG_BASE_PATH}/page`,
    {
      query,
    },
  );
}

export function getSystemConfigDetail(id: number) {
  return http.get<SystemConfigRecord>(`${SYSTEM_CONFIG_BASE_PATH}/${id}`);
}

export function getSystemConfigByKey(configKey: string, forceRefresh = false) {
  const normalizedKey = configKey.trim();

  if (!normalizedKey) {
    return Promise.resolve(null);
  }

  if (forceRefresh || !systemConfigValueCache.has(normalizedKey)) {
    systemConfigValueCache.set(
      normalizedKey,
      http.get<SystemConfigValueRecord>(
        `${SYSTEM_CONFIG_BASE_PATH}/key/${encodeURIComponent(normalizedKey)}`,
      ),
    );
  }

  return systemConfigValueCache.get(normalizedKey) as Promise<SystemConfigValueRecord>;
}

export function clearSystemConfigCache(configKey?: string) {
  if (configKey) {
    systemConfigValueCache.delete(configKey.trim());
    return;
  }

  systemConfigValueCache.clear();
}

export function createSystemConfig(data: SystemConfigCreateRequest) {
  clearSystemConfigCache(data.configKey);
  return http.post<void>(SYSTEM_CONFIG_BASE_PATH, data);
}

export function updateSystemConfig(
  id: number,
  data: SystemConfigUpdateRequest,
) {
  clearSystemConfigCache(data.configKey);
  return http.put<void>(`${SYSTEM_CONFIG_BASE_PATH}/${id}`, data);
}

export function deleteSystemConfig(id: number) {
  clearSystemConfigCache();
  return http.delete<void>(`${SYSTEM_CONFIG_BASE_PATH}/${id}`);
}

export function batchDeleteSystemConfigs(data: SystemConfigBatchDeleteRequest) {
  clearSystemConfigCache();
  return http.post<void>(`${SYSTEM_CONFIG_BASE_PATH}/batch-delete`, data);
}

export function updateSystemConfigStatus(
  id: number,
  data: SystemConfigStatusRequest,
) {
  clearSystemConfigCache();
  return http.patch<void>(`${SYSTEM_CONFIG_BASE_PATH}/${id}/status`, data);
}
