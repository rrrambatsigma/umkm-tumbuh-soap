import { getAccessToken } from "../auth/currentUser";

// Vite dapat mengganti URL ini dengan proxy lokal saat pengembangan.
const API_BASE_URL =
  import.meta.env.VITE_API_BASE_URL ?? "http://localhost:8082/api/v1";

console.log("[HTTP Client] API_BASE_URL:", API_BASE_URL);

const AUTH_API_BASE_URL =
  import.meta.env.VITE_AUTH_API_BASE_URL ??
  "http://localhost:8080/api/v1";

const ADMIN_API_BASE_URL =
  import.meta.env.VITE_ADMIN_API_BASE_URL ??
  "http://localhost:8080/api/v1";

const USER_API_BASE_URL =
  import.meta.env.VITE_USER_API_BASE_URL ??
  import.meta.env.VITE_USER_SERVICE_URL ??
  "http://localhost:8081/api/v1";

const PARTNERSHIP_API_BASE_URL =
  import.meta.env.VITE_PARTNERSHIP_API_BASE_URL ??
  "http://localhost:8082/api/v1";

const DOCUMENT_API_BASE_URL =
  import.meta.env.VITE_DOCUMENT_API_BASE_URL ??
  "http://localhost:8083/api/v1";

// ⭐ EXPORT base URL sebagai string — dipakai langsung di template literal
// oleh file-file seperti documents.ts: `${USER_API}/profile/...`
export const AUTH_API = AUTH_API_BASE_URL;
export const ADMIN_API = ADMIN_API_BASE_URL;
export const USER_API = USER_API_BASE_URL;
export const PARTNERSHIP_API = PARTNERSHIP_API_BASE_URL;
export const DOCUMENT_API = DOCUMENT_API_BASE_URL;

export type ServiceName =
  | "default"
  | "auth"
  | "admin"
  | "user"
  | "partnership"
  | "document";

export type RequestOptions = RequestInit & {
  auth?: boolean;
  service?: ServiceName;
  skipJsonContentType?: boolean;
  timeoutMs?: number;
};

export type ErrorPayload = {
  error?: string;
  message?: string;
  details?: unknown;
  [key: string]: unknown;
};

export class ApiError extends Error {
  status: number;
  payload: unknown;

  constructor(status: number, message: string, payload?: unknown) {
    super(message);
    this.name = "ApiError";
    this.status = status;
    this.payload = payload;
  }
}

function getPayloadMessage(payload: unknown) {
  if (!payload || typeof payload !== "object") return "";

  const candidate = payload as ErrorPayload;

  if (typeof candidate.error === "string" && candidate.error.trim()) {
    return candidate.error.trim();
  }

  if (typeof candidate.message === "string" && candidate.message.trim()) {
    return candidate.message.trim();
  }

  return "";
}

function getFallbackErrorMessage(status: number) {
  switch (status) {
    case 0:
      return "Tidak dapat terhubung ke server. Periksa koneksi Anda lalu coba lagi.";
    case 400:
      return "Data yang dikirim belum valid. Periksa kembali isian formulir.";
    case 401:
      return "Sesi tidak valid atau sudah berakhir. Silakan login kembali.";
    case 403:
      return "Anda tidak memiliki akses untuk melakukan aksi ini.";
    case 404:
      return "Data atau endpoint tidak ditemukan.";
    case 409:
      return "Data sudah digunakan atau bertabrakan dengan data yang ada.";
    case 413:
      return "Ukuran file terlalu besar.";
    case 422:
      return "Validasi data gagal. Periksa kembali isian formulir.";
    case 500:
      return "Server sedang bermasalah. Coba lagi beberapa saat.";
    case 502:
    case 503:
    case 504:
      return "Service backend sedang tidak tersedia. Coba lagi beberapa saat.";
    default:
      return "Terjadi kesalahan pada server.";
  }
}

function getHttpErrorMessage(status: number, payload: unknown) {
  return getPayloadMessage(payload) || getFallbackErrorMessage(status);
}

function getBaseURL(service: ServiceName = "default"): string {
  switch (service) {
    case "auth":
      return AUTH_API_BASE_URL;
    case "admin":
      return ADMIN_API_BASE_URL;
    case "default":
      return API_BASE_URL;
    case "user":
      return USER_API_BASE_URL;
    case "partnership":
      return PARTNERSHIP_API_BASE_URL;
    case "document":
      return DOCUMENT_API_BASE_URL;
    default:
      return API_BASE_URL;
  }
}

function shouldUseJsonContentType(
  body: BodyInit | null | undefined,
  skipJsonContentType: boolean,
) {
  if (!body) return false;
  if (skipJsonContentType) return false;
  if (body instanceof FormData) return false;
  return true;
}

function serializeBody(body: unknown): BodyInit | null | undefined {
  if (body === undefined || body === null) return undefined;

  if (
    body instanceof FormData ||
    body instanceof Blob ||
    body instanceof ArrayBuffer ||
    body instanceof URLSearchParams ||
    typeof body === "string"
  ) {
    return body;
  }

  return JSON.stringify(body);
}

export function createHttpClient(baseUrl: string) {
  async function clientRequest<T>(
    path: string,
    options: RequestOptions = {},
  ): Promise<T> {
    return requestWithBaseURL<T>(baseUrl, path, options);
  }

  function httpRequest<T>(
    path: string,
    options: RequestOptions = {},
  ): Promise<T> {
    return clientRequest<T>(path, options);
  }

  return Object.assign(httpRequest, {
    get<T>(path: string, options?: RequestOptions) {
      return clientRequest<T>(path, {
        ...options,
        method: "GET",
      });
    },

    post<T>(path: string, body?: unknown, options?: RequestOptions) {
      return clientRequest<T>(path, {
        ...options,
        method: "POST",
        body: serializeBody(body),
      });
    },

    put<T>(path: string, body?: unknown, options?: RequestOptions) {
      return clientRequest<T>(path, {
        ...options,
        method: "PUT",
        body: serializeBody(body),
      });
    },

    patch<T>(path: string, body?: unknown, options?: RequestOptions) {
      return clientRequest<T>(path, {
        ...options,
        method: "PATCH",
        body: serializeBody(body),
      });
    },

    delete<T>(path: string, options?: RequestOptions) {
      return clientRequest<T>(path, {
        ...options,
        method: "DELETE",
      });
    },
  });
}

export async function apiRequest<T>(
  path: string,
  options: RequestOptions = {},
): Promise<T> {
  const { service = "default", ...rest } = options;
  return requestWithBaseURL<T>(getBaseURL(service), path, rest);
}

async function requestWithBaseURL<T>(
  baseUrl: string,
  path: string,
  options: RequestOptions = {},
): Promise<T> {
  const {
    auth = true,
    headers,
    skipJsonContentType = false,
    timeoutMs,
    ...rest
  } = options;

  delete rest.service;

  const requestHeaders = new Headers(headers);
  const body = rest.body;

  if (
    !requestHeaders.has("Content-Type") &&
    shouldUseJsonContentType(body, skipJsonContentType)
  ) {
    requestHeaders.set("Content-Type", "application/json");
  }

  if (auth) {
    const token = getAccessToken();

    if (token) {
      requestHeaders.set("Authorization", `Bearer ${token}`);
    }
  }

  const controller = timeoutMs === undefined ? undefined : new AbortController();
  const signal = controller
    ? rest.signal
      ? AbortSignal.any([rest.signal, controller.signal])
      : controller.signal
    : rest.signal;
  const timeout = controller
    ? setTimeout(() => {
        controller.abort(new DOMException("Request timed out", "TimeoutError"));
      }, timeoutMs)
    : undefined;

  try {
    const response = await fetch(`${baseUrl}${path}`, {
      ...rest,
      headers: requestHeaders,
      signal,
    });

    const contentType = response.headers.get("content-type") ?? "";
    const isJson = contentType.includes("application/json");

    let payload: unknown = null;

    if (isJson) {
      payload = await response.json().catch(() => null);
    } else if (!response.ok) {
      const text = await response.text().catch(() => "");
      payload = text ? { message: text } : null;
    }

    // Abort saat membaca body tetap harus dilaporkan sebagai kegagalan.
    signal?.throwIfAborted();

    if (!response.ok) {
      throw new ApiError(
        response.status,
        getHttpErrorMessage(response.status, payload),
        payload,
      );
    }

    return payload as T;
  } catch (err) {
    if (err instanceof ApiError) throw err;

    if (signal?.aborted && signal.reason?.name === "TimeoutError") {
      throw new ApiError(
        0,
        "Server belum merespons dalam batas waktu. Silakan coba lagi.",
        err,
      );
    }

    throw new ApiError(0, getFallbackErrorMessage(0), err);
  } finally {
    clearTimeout(timeout);
  }
}

export const http = Object.assign(
  function httpRequest<T>(
    path: string,
    options: RequestOptions = {},
  ): Promise<T> {
    return apiRequest<T>(path, options);
  },
  {
    get<T>(path: string, options?: RequestOptions) {
      return apiRequest<T>(path, {
        ...options,
        method: "GET",
      });
    },

    post<T>(path: string, body?: unknown, options?: RequestOptions) {
      return apiRequest<T>(path, {
        ...options,
        method: "POST",
        body: serializeBody(body),
      });
    },

    put<T>(path: string, body?: unknown, options?: RequestOptions) {
      return apiRequest<T>(path, {
        ...options,
        method: "PUT",
        body: serializeBody(body),
      });
    },

    patch<T>(path: string, body?: unknown, options?: RequestOptions) {
      return apiRequest<T>(path, {
        ...options,
        method: "PATCH",
        body: serializeBody(body),
      });
    },

    delete<T>(path: string, options?: RequestOptions) {
      return apiRequest<T>(path, {
        ...options,
        method: "DELETE",
      });
    },
  },
);
