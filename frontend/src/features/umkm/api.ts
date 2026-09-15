import { http } from "../../shared/api/http";

export type UmkmProfile = {
  id: string;
  user_id: string;
  name: string;
  category: string | null;
  description: string | null;
  person: string | null;
  phone_number: string | null;
  address: string | null;
  city: string | null;
  province: string | null;
  omzet: number | null;
  created_at: string;
  updated_at: string;
};

export type UmkmProfilePayload = {
  business_name: string;
  business_category?: string;
  business_description?: string;
  owner_name?: string;
  phone_number?: string;
  address?: string;
  city?: string;
  province?: string;
  omzet?: number;
};

export function getProfile() {
  return http<{ profile: UmkmProfile }>("/profiles/me", { service: "user" });
}

export function updateProfile(payload: UmkmProfilePayload) {
  return http<{ profile: UmkmProfile }>("/profiles/me", {
    method: "PUT",
    body: JSON.stringify(payload),
    service: "user",
  });
}
