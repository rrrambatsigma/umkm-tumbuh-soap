// frontend/src/shared/api/soapClient.ts

// Klien SOAP 1.1 untuk soap-partnerships-service (POST /partnership).
// Parser berbasis tag-scan (tanpa DOMParser) agar dapat diuji langsung di Node.

import { getAccessToken } from "../auth/currentUser";
import { ApiError } from "./http";

export const SOAP_NAMESPACE = "http://umkm-tumbuh.example.com/partnerships";

const SOAP_API_BASE_URL: string =
  import.meta.env.VITE_SOAP_API_BASE_URL ?? "http://localhost:9090/partnership";

export type SoapFields = Record<
  string,
  string | number | boolean | null | undefined
>;

export type XmlNode = {
  name: string;
  text: string;
  children: XmlNode[];
};

const TAG_PATTERN = /<(\/?)([A-Za-z_][\w.:-]*)([^>]*?)(\/?)>([^<]*)/g;

export function escapeXml(value: string): string {
  return value
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;")
    .replace(/"/g, "&quot;")
    .replace(/'/g, "&apos;");
}

export function buildEnvelope(operation: string, fields: SoapFields): string {
  const children = Object.entries(fields)
    .filter(([, value]) => value !== undefined && value !== null && value !== "")
    .map(([key, value]) => `      <${key}>${escapeXml(String(value))}</${key}>`)
    .join("\n");

  return [
    '<?xml version="1.0" encoding="UTF-8"?>',
    `<soapenv:Envelope xmlns:soapenv="http://schemas.xmlsoap.org/soap/envelope/" xmlns:tns="${SOAP_NAMESPACE}">`,
    "  <soapenv:Header/>",
    "  <soapenv:Body>",
    `    <${operation}>`,
    children,
    `    </${operation}>`,
    "  </soapenv:Body>",
    "</soapenv:Envelope>",
  ].join("\n");
}

function localName(name: string): string {
  const index = name.indexOf(":");
  return index === -1 ? name : name.slice(index + 1);
}

function decodeEntities(text: string): string {
  return text
    .replace(/&#x([0-9a-fA-F]+);/g, (_, hex: string) =>
      String.fromCharCode(Number.parseInt(hex, 16)),
    )
    .replace(/&#(\d+);/g, (_, decimal: string) =>
      String.fromCharCode(Number(decimal)),
    )
    .replace(/&lt;/g, "<")
    .replace(/&gt;/g, ">")
    .replace(/&quot;/g, '"')
    .replace(/&apos;/g, "'")
    .replace(/&amp;/g, "&");
}

export function parseXmlTree(xml: string): XmlNode {
  const root: XmlNode = { name: "", text: "", children: [] };
  const stack: XmlNode[] = [root];
  let match: RegExpExecArray | null;

  TAG_PATTERN.lastIndex = 0;

  while ((match = TAG_PATTERN.exec(xml)) !== null) {
    const closing = match[1];
    const rawName = match[2];
    const selfClosing = match[4];
    const text = match[5];

    if (closing) {
      if (stack.length > 1) stack.pop();
      continue;
    }

    const node: XmlNode = {
      name: localName(rawName),
      text: decodeEntities(text),
      children: [],
    };
    stack[stack.length - 1].children.push(node);

    if (!selfClosing) stack.push(node);
  }

  return root;
}

export function findNode(root: XmlNode, name: string): XmlNode | undefined {
  if (root.name === name) return root;

  for (const child of root.children) {
    const found = findNode(child, name);
    if (found) return found;
  }

  return undefined;
}

function nodeToObject(node: XmlNode): unknown {
  if (node.children.length === 0) return node.text;

  const result: Record<string, unknown> = {};

  for (const child of node.children) {
    const value = nodeToObject(child);

    if (child.name in result) {
      const current = result[child.name];
      result[child.name] = Array.isArray(current)
        ? [...current, value]
        : [current, value];
    } else {
      result[child.name] = value;
    }
  }

  return result;
}

export function findSoapFault(xml: string): string | null {
  const fault = findNode(parseXmlTree(xml), "Fault");
  if (!fault) return null;

  const faultString = findNode(fault, "faultstring");
  const message = faultString?.text.trim();

  return message || "Permintaan SOAP gagal";
}

export function parseSoapResponse(xml: string): Record<string, unknown> {
  const body = findNode(parseXmlTree(xml), "Body");
  const operationResult = body?.children[0];

  if (!operationResult) {
    throw new ApiError(500, "Respons SOAP tidak berisi hasil operasi", xml);
  }

  const value = nodeToObject(operationResult);

  if (!value || typeof value !== "object") return {};

  return value as Record<string, unknown>;
}

function fallbackMessage(status: number) {
  switch (status) {
    case 0:
      return "Tidak dapat terhubung ke server. Periksa koneksi Anda lalu coba lagi.";
    case 401:
      return "Sesi tidak valid atau sudah berakhir. Silakan login kembali.";
    case 403:
      return "Anda tidak memiliki akses untuk melakukan aksi ini.";
    case 404:
      return "Data atau endpoint tidak ditemukan.";
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

export async function callSoap<T>(
  operation: string,
  fields: SoapFields,
): Promise<T> {
  const headers: Record<string, string> = {
    "Content-Type": "text/xml; charset=utf-8",
    SOAPAction: `${SOAP_NAMESPACE}/${operation}`,
  };
  const token = getAccessToken();

  if (token) headers.Authorization = `Bearer ${token}`;

  let response: Response;

  try {
    response = await fetch(SOAP_API_BASE_URL, {
      method: "POST",
      headers,
      body: buildEnvelope(operation, fields),
    });
  } catch (err) {
    throw new ApiError(0, fallbackMessage(0), err);
  }

  const xml = await response.text().catch(() => "");
  const fault = findSoapFault(xml);

  if (fault) throw new ApiError(response.status || 400, fault, { fault });
  if (!response.ok) throw new ApiError(response.status, fallbackMessage(response.status), xml);

  return parseSoapResponse(xml) as T;
}
