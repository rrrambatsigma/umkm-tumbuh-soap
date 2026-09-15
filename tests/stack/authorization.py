"""Authorization regressions against the real, isolated Compose services."""

import base64
import hashlib
import hmac
import json
import os
import time
import unittest
from urllib.error import HTTPError
from urllib.request import Request, urlopen

from check import AUTH, DOCUMENT, PNG, upload


PARTNERSHIP = "http://partnerships-service:8082/api/v1"


def request(url, method="GET", data=None, token=None, expected=200, headers=None):
    headers = dict(headers or {})
    if token:
        headers["Authorization"] = "Bearer " + token
    if data is not None:
        headers["Content-Type"] = "application/json"
        data = json.dumps(data).encode()
    try:
        with urlopen(Request(url, data=data, headers=headers, method=method), timeout=15) as response:
            status, body = response.status, response.read()
    except HTTPError as error:
        status, body = error.code, error.read()
    if status != expected:
        detail = "[login response omitted]" if url.endswith("/auth/login") else body.decode(errors="replace")[:1000]
        raise AssertionError(f"{method} {url}: expected {expected}, got {status}: {detail}")
    if body.startswith(b"%PDF"):
        return body
    return json.loads(body) if body else None


def signed_token(claims, algorithm="HS256", key=None):
    def encode(value):
        return base64.urlsafe_b64encode(json.dumps(value).encode()).rstrip(b"=")
    content = encode({"alg": algorithm, "typ": "JWT"}) + b"." + encode(claims)
    if algorithm == "none":
        signature = b""
    else:
        digest = hashlib.sha384 if algorithm == "HS384" else hashlib.sha256
        signature = base64.urlsafe_b64encode(hmac.new((key or os.environ["JWT_SECRET"]).encode(), content, digest).digest()).rstrip(b"=")
    return (content + b"." + signature).decode()


class AuthorizationTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.tokens = {}
        for account in ("admin", "umkm.a", "umkm.b", "mitra.a", "mitra.b"):
            result = request(AUTH + "/auth/login", "POST", {
                "email": account + "@stage1.test", "password": "Stage1Test123!",
            })
            cls.tokens[account] = result["access_token"]
        cls.documents = {}
        for suffix in ("a", "b"):
            cls.documents[suffix] = upload(DOCUMENT + "/documents/upload", cls.tokens["umkm." + suffix],
                "file", "authorization.png", "image/png", PNG,
                {"category": "PARTNERSHIP_FILE"}, expected=201)["document"]["id"]
        cls.partnership_id = cls.create_partnership()

    @classmethod
    def create_partnership(cls):
        result = request(PARTNERSHIP + "/partnerships", "POST", {
            "receiver_id": "TEST_PARTNER_A", "proposal_title": "Pengajuan uji otorisasi",
            "proposal_description": "Usulan kerja sama sintetis untuk pengujian izin akses aplikasi.",
            "attachment_files": [cls.documents["a"]], "requester_id": "TEST_UMKM_B",
        }, cls.tokens["umkm.a"])
        return result["data"]["pengajuanID"]

    def test_invalid_tokens_and_role_headers_are_rejected(self):
        claims = {"sub": "TEST_UMKM_A", "role": "UMKM", "exp": int(time.time()) + 3600}
        tokens = {
            "forged": signed_token(claims, key="not-the-signing-key"),
            "expired": signed_token({**claims, "exp": int(time.time()) - 60}),
            "unsigned": signed_token(claims, algorithm="none"),
            "wrong algorithm": signed_token(claims, algorithm="HS384"),
            "missing expiry": signed_token({"sub": "TEST_UMKM_A", "role": "UMKM"}),
            "empty subject": signed_token({**claims, "sub": ""}),
        }
        url = PARTNERSHIP + "/partnerships/summary"
        for name, token in tokens.items():
            with self.subTest(url=url, token_kind=name):
                request(url, token=token, expected=401)
            request(url, headers={"X-User-Role": "ADMIN"}, expected=401)
        request(PARTNERSHIP + "/umkm", token=self.tokens["umkm.a"], headers={"X-User-Role": "MITRA"}, expected=403)
        request(PARTNERSHIP + "/mitra", token=self.tokens["umkm.a"], headers={"X-User-Role": "MITRA"})
        request(PARTNERSHIP + "/umkm", token=self.tokens["mitra.a"])

    def test_partnership_details_and_mutations_reject_other_accounts(self):
        url = PARTNERSHIP + "/partnerships/" + self.partnership_id
        for role in ("umkm.b", "mitra.b"):
            for method, suffix, body in (
                ("GET", "", None), ("POST", "/sign", {"dokumen_kontrak": "TEST_CONTRACT_B"}),
                ("PATCH", "/read", {}), ("PATCH", "/approve", {}),
                ("PATCH", "/reject", {"rejection_reason": "Test"}), ("PATCH", "/cancel", {}),
            ):
                with self.subTest(account=role, route=suffix):
                    request(url + suffix, method, body, self.tokens[role], expected=403)
        detail = request(url, token=self.tokens["umkm.a"])["data"]["pengajuan"]
        self.assertEqual(detail["requester_id"], "TEST_UMKM_A")
        self.assertEqual(detail["status"], "DIAJUKAN")
        request(url, token=self.tokens["mitra.a"])

    def test_partnership_roles_and_state_transitions(self):
        url = PARTNERSHIP + "/partnerships/" + self.create_partnership()
        request(url + "/approve", "PATCH", {}, self.tokens["umkm.a"], expected=403)
        request(url + "/cancel", "PATCH", {}, self.tokens["mitra.a"], expected=403)
        request(url + "/read", "PATCH", {}, self.tokens["umkm.a"], expected=403)
        request(url + "/sign", "POST", {"dokumen_kontrak": "TEST_CONTRACT_A"}, self.tokens["mitra.a"], expected=403)
        request(url + "/sign", "POST", {"dokumen_kontrak": "TEST_CONTRACT_B"}, self.tokens["umkm.a"], expected=403)
        request(url + "/sign", "POST", {"dokumen_kontrak": "TEST_CONTRACT_A"}, self.tokens["umkm.a"])
        request(url + "/read", "PATCH", {}, self.tokens["mitra.a"])
        request(url + "/approve", "PATCH", {}, self.tokens["mitra.a"])
        request(url + "/approve", "PATCH", {}, self.tokens["mitra.a"], expected=409)
        request(url + "/cancel", "PATCH", {}, self.tokens["umkm.a"], expected=409)
        request(url + "/sign", "POST", {"dokumen_kontrak": "TEST_CONTRACT_A"}, self.tokens["umkm.a"], expected=409)
        self.assertEqual(request(url, token=self.tokens["umkm.a"])["data"]["pengajuan"]["status"], "AKTIF")
        for operation, account, payload, expected_status in (
            ("cancel", "umkm.a", {}, "DIBATALKAN"),
            ("reject", "mitra.a", {"rejection_reason": "Tidak sesuai kebutuhan"}, "DITOLAK"),
        ):
            other = PARTNERSHIP + "/partnerships/" + self.create_partnership()
            request(other + "/" + operation, "PATCH", payload, self.tokens[account])
            self.assertEqual(request(other, token=self.tokens["umkm.a"])["data"]["pengajuan"]["status"], expected_status)

    def test_foreign_attachment_does_not_create_a_partnership(self):
        token = self.tokens["umkm.a"]
        listing = PARTNERSHIP + "/partnerships/status"
        before = request(listing, token=token)["data"]["pagination"]["total"]
        request(PARTNERSHIP + "/partnerships", "POST", {
            "receiver_id": "TEST_PARTNER_A", "proposal_title": "Pengajuan lampiran asing",
            "proposal_description": "Pengajuan ini harus ditolak sebelum menyimpan data kemitraan.",
            "attachment_files": [self.documents["b"]],
        }, token, expected=403)
        self.assertEqual(request(listing, token=token)["data"]["pagination"]["total"], before)


if __name__ == "__main__":
    unittest.main(verbosity=2)
