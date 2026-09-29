// frontend/src/features/partnerships/soapApi.ts

// Adapter untuk transport SOAP. Setiap fungsi memanggil
// soap-partnerships-service lalu menormalkan hasilnya ke bentuk yang sama
// dengan respons REST partnerships-service, sehingga halaman kemitraan tidak
// perlu tahu transport yang sedang dipakai.

import { ApiError } from "../../shared/api/http";
import { callSoap } from "../../shared/api/soapClient";
import { getCurrentUser } from "../../shared/auth/currentUser";
import type {
  IncomingPartnershipSummaryResponse,
  IncomingPartnershipsResponse,
  PartnershipStatusResponse,
} from "./api";
import { normalizeStatusFilter } from "./statusFilter";
import type { CreatePartnershipRequest, PartnershipStatus } from "./types";

type ListParams = {
  page?: number;
  limit?: number;
  status?: string;
};

type StatusResponse = {
  success: boolean;
  message?: string;
  data: PartnershipStatusResponse;
};

type IncomingResponse = {
  success: boolean;
  message?: string;
  data: IncomingPartnershipsResponse;
};

type ActionResponse = {
  success: boolean;
  message?: string;
  data: void;
};

function identity(): { userId: string; userRole: string } {
  const user = getCurrentUser();

  if (!user?.id || !user?.role) {
    throw new ApiError(401, "Sesi tidak valid atau sudah berakhir. Silakan login kembali.");
  }

  return { userId: user.id, userRole: user.role };
}

function asRecord(value: unknown): Record<string, unknown> {
  if (value && typeof value === "object" && !Array.isArray(value)) {
    return value as Record<string, unknown>;
  }

  return {};
}

function asString(value: unknown): string {
  if (value === undefined || value === null) return "";
  return typeof value === "string" ? value : String(value);
}

function asNumber(value: unknown): number {
  const parsed = Number(value);
  return Number.isFinite(parsed) ? parsed : 0;
}

function asBool(value: unknown): boolean {
  return value === true || value === "true";
}

function toArray<T>(value: unknown): T[] {
  if (value === undefined || value === null || value === "") return [];
  return Array.isArray(value) ? (value as T[]) : [value as T];
}

function normalizePage(page?: number): number {
  return page && page > 0 ? Math.floor(page) : 1;
}

function normalizeLimit(limit?: number): number {
  return limit && limit > 0 && limit <= 100 ? Math.floor(limit) : 10;
}

// Meniru pembagian bulat pada REST: (total + limit - 1) / limit.
function totalPages(total: number, limit: number): number {
  return Math.floor((total + limit - 1) / limit);
}

// Meniru formatPartnershipListTitle pada handler REST.
function formatListTitle(businessName: string, requesterName: string): string {
  const name = businessName.trim() || requesterName.trim();
  return name ? `Pengajuan Kemitraan - ${name}` : "Pengajuan Kemitraan";
}

// SOAP menyimpan judul dan deskripsi proposal dalam satu kolom teks yang
// dipisahkan dua baris baru, sesuai urutan payload pembuatan.
function splitProposalText(text: string): { title: string; description: string } {
  const separator = text.indexOf("\n\n");

  if (separator === -1) return { title: text.trim(), description: "" };

  return {
    title: text.slice(0, separator).trim(),
    description: text.slice(separator + 2),
  };
}

function countByStatus(items: unknown): Record<string, number> {
  const counts: Record<string, number> = {};

  for (const raw of toArray<unknown>(items)) {
    const item = asRecord(raw);
    const status = asString(item.status);
    counts[status] = (counts[status] ?? 0) + asNumber(item.count);
  }

  return counts;
}

function extractSummaryItems(response: Record<string, unknown>): unknown {
  const summary = response.summary;

  if (!summary || typeof summary !== "object") return undefined;

  return asRecord(summary).item;
}

function emptyStatus(page: number, limit: number): StatusResponse {
  return {
    success: true,
    message: "",
    data: {
      pengajuan: [],
      pagination: { page, limit, total: 0, totalPages: 0 },
    },
  };
}

function emptyIncoming(page: number, limit: number): IncomingResponse {
  return {
    success: true,
    message: "",
    data: {
      pengajuan_masuk: [],
      pagination: { page, limit, total: 0, totalPages: 0 },
    },
  };
}

export async function create(
  data: CreatePartnershipRequest,
): Promise<{ success: boolean; message: string; data: { pengajuanID: string } }> {
  const { userId, userRole } = identity();
  const result = asRecord(
    await callSoap("CreatePartnershipApplication", {
      userId,
      userRole,
      receiverId: data.receiver_id,
      proposalTitle: data.proposal_title,
      proposalDescription: data.proposal_description,
    }),
  );

  return {
    success: asBool(result.success),
    message: asString(result.message),
    data: { pengajuanID: asString(result.applicationId) },
  };
}

export async function getStatus(params?: ListParams): Promise<StatusResponse> {
  const { userId, userRole } = identity();
  const page = normalizePage(params?.page);
  const limit = normalizeLimit(params?.limit);
  const status = normalizeStatusFilter(params?.status);

  // Status yang tidak ada di database tidak dikirim ke SOAP agar hasilnya
  // sama dengan REST (daftar kosong, bukan fault 400).
  if (params?.status?.trim() && !status) return emptyStatus(page, limit);

  const response = asRecord(
    await callSoap("GetPartnershipApplications", {
      userId,
      userRole,
      status,
      page,
      limit,
    }),
  );
  const items = toArray<unknown>(asRecord(response.applications).application);
  const total = asNumber(response.totalCount);

  return {
    success: true,
    message: "",
    data: {
      pengajuan: items.map((raw) => {
        const item = asRecord(raw);

        return {
          pengajuanID: asString(item.applicationId),
          statusPengajuan: asString(item.status) as PartnershipStatus,
          tanggalPengajuan: asString(item.submittedAt),
          mitraUmkmTujuan: asString(item.receiverName),
          mitraUmkmUsaha: asString(item.receiverBusinessName),
          pengirim: asString(item.requesterName),
          proposalTitle: formatListTitle(
            asString(item.requesterBusinessName),
            asString(item.requesterName),
          ),
        };
      }),
      pagination: { page, limit, total, totalPages: totalPages(total, limit) },
    },
  };
}

export async function getIncoming(params?: ListParams): Promise<IncomingResponse> {
  const { userId, userRole } = identity();
  const page = normalizePage(params?.page);
  const limit = normalizeLimit(params?.limit);
  const status = normalizeStatusFilter(params?.status);

  if (params?.status?.trim() && !status) return emptyIncoming(page, limit);

  const response = asRecord(
    await callSoap("GetIncomingPartnerships", {
      userId,
      userRole,
      status,
      page,
      limit,
    }),
  );
  const items = toArray<unknown>(asRecord(response.applications).application);
  const total = asNumber(response.totalCount);

  return {
    success: true,
    message: "",
    data: {
      pengajuan_masuk: items.map((raw) => {
        const item = asRecord(raw);

        return {
          pengajuanID: asString(item.applicationId),
          pengirim: asString(item.requesterName),
          proposal_title: formatListTitle(
            asString(item.requesterBusinessName),
            asString(item.requesterName),
          ),
          tanggalPengajuan: asString(item.submittedAt),
          status: asString(item.status),
        };
      }),
      pagination: { page, limit, total, totalPages: totalPages(total, limit) },
    },
  };
}

export async function getSummary(): Promise<{
  success: boolean;
  message?: string;
  data: { summary: { bermitra: number; menunggu: number; ditolak: number } };
}> {
  const { userId, userRole } = identity();
  const response = asRecord(
    await callSoap("GetPartnershipSummary", { userId, userRole }),
  );
  const counts = countByStatus(extractSummaryItems(response));

  return {
    success: true,
    message: "",
    data: {
      summary: {
        bermitra: (counts.AKTIF ?? 0) + (counts.SELESAI ?? 0),
        menunggu:
          (counts.DIAJUKAN ?? 0) +
          (counts.DITINJAU ?? 0) +
          (counts.MENUNGGU_DOKUMEN_TTD ?? 0),
        ditolak: (counts.DITOLAK ?? 0) + (counts.DIBATALKAN ?? 0),
      },
    },
  };
}

export async function getIncomingSummary(): Promise<{
  success: boolean;
  message?: string;
  data: IncomingPartnershipSummaryResponse;
}> {
  const { userId, userRole } = identity();
  const response = asRecord(
    await callSoap("GetIncomingPartnershipSummary", { userId, userRole }),
  );
  const counts = countByStatus(extractSummaryItems(response));
  const total = Object.values(counts).reduce((sum, value) => sum + value, 0);

  return {
    success: true,
    message: "",
    data: {
      summary: {
        menunggu: (counts.DIAJUKAN ?? 0) + (counts.DITINJAU ?? 0),
        disetujui:
          (counts.MENUNGGU_DOKUMEN_TTD ?? 0) +
          (counts.APPROVED ?? 0) +
          (counts.AKTIF ?? 0) +
          (counts.SELESAI ?? 0),
        ditolak: counts.DITOLAK ?? 0,
        dibatalkan: counts.DIBATALKAN ?? 0,
        total,
      },
    },
  };
}

export async function getDetail(id: string): Promise<{
  success: boolean;
  message?: string;
  data: Record<string, unknown>;
}> {
  const { userId, userRole } = identity();
  const response = asRecord(
    await callSoap("GetPartnershipApplication", {
      userId,
      userRole,
      applicationId: id,
    }),
  );
  const proposal = splitProposalText(asString(response.proposalText));

  return {
    success: true,
    message: "",
    data: {
      pengajuan: {
        id: asString(response.applicationId),
        request_code: asString(response.requestCode),
        requester_id: asString(response.requesterId),
        receiver_id: asString(response.receiverId),
        requester_name: asString(response.requesterName),
        receiver_name: asString(response.receiverName),
        proposal_title: proposal.title,
        proposal_description: proposal.description,
        status: asString(response.status),
        rejection_reason: asString(response.rejectionReason) || null,
        contract_document_id: asString(response.contractDocumentId) || null,
        submitted_at: asString(response.submittedAt) || null,
        decided_at: asString(response.decidedAt) || null,
        created_at: asString(response.createdAt),
        updated_at: asString(response.updatedAt),
        // SOAP tidak menyediakan operasi lampiran; daftar lampiran dikosongkan.
        attachments: [],
      },
    },
  };
}

async function updateStatus(
  id: string,
  status: "AKTIF" | "DITOLAK" | "DIBATALKAN",
  rejectionReason?: string,
): Promise<ActionResponse> {
  const { userId, userRole } = identity();
  const response = asRecord(
    await callSoap("UpdatePartnershipStatus", {
      userId,
      userRole,
      applicationId: id,
      status,
      rejectionReason: rejectionReason ?? "",
    }),
  );

  return {
    success: asBool(response.success) || asString(response.status) === status,
    message: asString(response.message),
    data: undefined,
  };
}

export function approve(id: string): Promise<ActionResponse> {
  return updateStatus(id, "AKTIF");
}

export function reject(id: string, rejectionReason: string): Promise<ActionResponse> {
  return updateStatus(id, "DITOLAK", rejectionReason);
}

export function cancel(id: string): Promise<ActionResponse> {
  return updateStatus(id, "DIBATALKAN");
}

export async function markAsRead(id: string): Promise<ActionResponse> {
  const { userId, userRole } = identity();
  const response = asRecord(
    await callSoap("MarkPartnershipAsRead", {
      userId,
      userRole,
      applicationId: id,
    }),
  );

  return {
    success: asBool(response.success),
    message: asString(response.message),
    data: undefined,
  };
}

export async function sign(id: string, dokumenKontrak: string): Promise<ActionResponse> {
  const { userId, userRole } = identity();
  const response = asRecord(
    await callSoap("SignPartnership", {
      userId,
      userRole,
      applicationId: id,
      documentId: dokumenKontrak,
    }),
  );

  return {
    success: asBool(response.success),
    message: asString(response.message),
    data: undefined,
  };
}
