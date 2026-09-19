export type ApiResponse<T> = {
  code: number;
  message: string;
  data: T;
};

export type ApiPageRequest = {
  page: number;
  pageSize: number;
};

export type ApiPageResult<T> = {
  records: T[];
  total: number;
  page: number;
  pageSize: number;
};

export type ApiFieldErrors = Record<string, string>;

export type ApiStatus = 0 | 1;

export type ApiBuiltinFlag = 0 | 1;

export type ApiQueryValue =
  | string
  | number
  | boolean
  | null
  | undefined
  | Array<string | number | boolean>;

export type ApiQueryParams = Record<string, ApiQueryValue>;
