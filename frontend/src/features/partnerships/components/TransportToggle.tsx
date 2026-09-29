// frontend/src/features/partnerships/components/TransportToggle.tsx

import { useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import {
  getPartnershipTransport,
  togglePartnershipTransport,
  type PartnershipTransport,
} from "../../../shared/api/transport";

// Tombol untuk berpindah transport API kemitraan (REST <-> SOAP) saat runtime.
// Cache query ikut dibersihkan agar data yang tampil memakai transport baru.
export default function TransportToggle() {
  const queryClient = useQueryClient();
  const [transport, setTransport] = useState<PartnershipTransport>(getPartnershipTransport);

  function handleChange() {
    setTransport(togglePartnershipTransport());
    queryClient.clear();
  }

  return (
    <button
      type="button"
      className={`partnership-transport-toggle ${transport}`}
      onClick={handleChange}
      title="Ganti transport API kemitraan (REST atau SOAP)"
      aria-label={`Transport kemitraan ${transport.toUpperCase()}. Klik untuk mengganti.`}
    >
      Kemitraan: <strong>{transport.toUpperCase()}</strong>
    </button>
  );
}
