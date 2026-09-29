// frontend/src/features/partnerships/statusFilter.ts

// Dropdown filter di halaman status/inbox memakai nama status berbahasa Inggris,
// sedangkan database memakai status berbahasa Inggris yang sudah dinormalisasi
// (DIAJUKAN, DITINJAU, dst). Terjemahan dipakai oleh kedua transport agar
// perilaku filter REST dan SOAP identik.

const STATUS_ALIASES: Record<string, string> = {
  SUBMITTED: "DIAJUKAN",
  REVIEWED: "DITINJAU",
  WAITING_DOCUMENT: "MENUNGGU_DOKUMEN_TTD",
  ACTIVE: "AKTIF",
  REJECTED: "DITOLAK",
  COMPLETED: "SELESAI",
  CANCELLED: "DIBATALKAN",
};

const CANONICAL_STATUSES = new Set([
  "DRAFT",
  "DIAJUKAN",
  "DITINJAU",
  "AKTIF",
  "DITOLAK",
  "DIBATALKAN",
  "MENUNGGU_DOKUMEN_TTD",
  "SELESAI",
]);

// Mengembalikan status database, atau undefined bila status tidak ada di
// database (contoh: APPROVED) sehingga hasilnya kosong di kedua transport.
export function normalizeStatusFilter(status?: string): string | undefined {
  const value = status?.trim().toUpperCase();
  if (!value) return undefined;
  if (CANONICAL_STATUSES.has(value)) return value;

  return STATUS_ALIASES[value];
}
