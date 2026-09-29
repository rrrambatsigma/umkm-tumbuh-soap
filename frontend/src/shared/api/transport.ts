// frontend/src/shared/api/transport.ts

// Memilih transport untuk fitur kemitraan: REST (partnerships-service) atau SOAP
// (soap-partnerships-service). Nilai awal berasal dari VITE_PARTNERSHIP_TRANSPORT,
// lalu dapat diganti saat runtime lewat tombol di topbar dan disimpan di localStorage.

export type PartnershipTransport = "rest" | "soap";

const STORAGE_KEY = "partnership_transport";

const DEFAULT_TRANSPORT: PartnershipTransport =
  import.meta.env.VITE_PARTNERSHIP_TRANSPORT === "soap" ? "soap" : "rest";

function normalizeTransport(value: unknown): PartnershipTransport | null {
  if (value === "rest" || value === "soap") return value;
  return null;
}

export function getPartnershipTransport(): PartnershipTransport {
  if (typeof window !== "undefined") {
    try {
      const stored = normalizeTransport(window.localStorage.getItem(STORAGE_KEY));
      if (stored) return stored;
    } catch {
      // localStorage dapat diblokir (private mode) - gunakan nilai default.
    }
  }

  return DEFAULT_TRANSPORT;
}

export function setPartnershipTransport(transport: PartnershipTransport): void {
  if (typeof window !== "undefined") {
    try {
      window.localStorage.setItem(STORAGE_KEY, transport);
    } catch {
      // Gagal menyimpan tidak menghalangi penggunaan transport untuk sesi ini.
    }
  }
}

export function togglePartnershipTransport(): PartnershipTransport {
  const next: PartnershipTransport =
    getPartnershipTransport() === "soap" ? "rest" : "soap";
  setPartnershipTransport(next);
  return next;
}
