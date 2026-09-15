"""Exercise the five APIs and both S3 consumers inside the isolated Compose stack."""

import base64
import hashlib
import json
import os
from pathlib import Path
import sys
import time
from urllib.error import HTTPError, URLError
from urllib.request import Request, urlopen


AUTH = "http://auth-service:8080/api/v1"
USER = "http://user-service:8081/api/v1"
DOCUMENT = "http://document-service:8083/api/v1"
GARAGE = "http://garage:3903/v2"
STATE = Path("/state/check.json")
TEXT = b"Stage 1 document: upload, download, and restart.\n"
PNG = base64.b64decode(
    "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+ip1sAAAAASUVORK5CYII="
)


def request(url, method="GET", data=None, token=None, content_type=None, expected=200):
    headers = {}
    if token:
        headers["Authorization"] = "Bearer " + token
    if isinstance(data, dict):
        data = json.dumps(data).encode()
        content_type = "application/json"
    if content_type:
        headers["Content-Type"] = content_type
    try:
        with urlopen(Request(url, data, headers, method=method), timeout=10) as response:
            body = response.read()
            if response.status != expected:
                raise AssertionError(f"{method} {url}: expected {expected}, got {response.status}")
            return body
    except HTTPError as error:
        raise AssertionError(f"{method} {url}: HTTP {error.code}: {error.read().decode()}") from error


def api(url, **kwargs):
    return json.loads(request(url, **kwargs))


def garage(endpoint, **kwargs):
    return api(GARAGE + "/" + endpoint, token=os.environ["GARAGE_ADMIN_TOKEN"], **kwargs)


def upload(url, token, field, filename, content_type, content, fields=None, expected=200):
    boundary = "stage1-multipart-boundary"
    body = bytearray()
    for name, value in (fields or {}).items():
        body.extend(f'--{boundary}\r\nContent-Disposition: form-data; name="{name}"\r\n\r\n{value}\r\n'.encode())
    body.extend(f'--{boundary}\r\nContent-Disposition: form-data; name="{field}"; filename="{filename}"\r\nContent-Type: {content_type}\r\n\r\n'.encode())
    body.extend(content)
    body.extend(f"\r\n--{boundary}--\r\n".encode())
    return api(url, method="POST", token=token, data=bytes(body),
               content_type=f"multipart/form-data; boundary={boundary}", expected=expected)


def ready():
    for url in [AUTH + "/health/db", USER + "/health/db", DOCUMENT + "/health",
                "http://partnerships-service:8082/health"]:
        deadline = time.monotonic() + 90
        while True:
            try:
                request(url)
                break
            except (AssertionError, URLError, TimeoutError):
                if time.monotonic() >= deadline:
                    raise
                time.sleep(2)
    print("PASS: all four services are ready", flush=True)


def login_fixtures():
    tokens = {}
    for account, role, account_id in [
        ("admin", "ADMIN", "TEST_ADMIN"),
        ("umkm.a", "UMKM", "TEST_UMKM_A"), ("umkm.b", "UMKM", "TEST_UMKM_B"),
        ("mitra.a", "MITRA", "TEST_MITRA_A"), ("mitra.b", "MITRA", "TEST_MITRA_B"),
        ("onboarding", "UMKM", "TEST_ONBOARDING"),
    ]:
        result = api(AUTH + "/auth/login", method="POST",
                     data={"email": account + "@stage1.test", "password": "Stage1Test123!"})
        assert result["user"]["role"] == role, account
        assert result["user"]["id"] == account_id, account
        tokens[account] = result["access_token"]
        if account == "onboarding":
            assert result["user"]["status"] == "MENUNGGU"
            registration = api(USER + "/register/status", token=tokens[account])
            assert registration["profile_complete"] is False
            assert registration["submitted"] is False
        elif account != "admin":
            assert result["user"]["status"] == "DISETUJUI"
            api(USER + "/profiles/me", token=tokens[account])
    print("PASS: six fixtures log in with the expected roles and registration states", flush=True)
    return tokens


def key_fingerprint():
    matches = [key for key in garage("ListKeys") if key["name"] == "UMKM App Key"]
    assert len(matches) == 1, "Expected exactly one application key"
    key = garage("GetKeyInfo?id=" + matches[0]["id"] + "&showSecretKey=true")
    return hashlib.sha256((key["accessKeyId"] + key["secretAccessKey"]).encode()).hexdigest()


def verify_uploads(state, token):
    assert request(DOCUMENT + "/documents/" + state["document_id"] + "/download", token=token) == TEXT
    assert request(USER + "/products/" + state["product_id"] + "/thumbnail", token=token) == PNG
    print("PASS: document and product image downloads match the uploaded bytes", flush=True)


def main():
    ready()
    tokens = login_fixtures()
    token = tokens["umkm.a"]
    if sys.argv[1] == "prepare":
        # An unrelated key must survive bootstrap reruns too.
        sentinel = garage("CreateKey", method="POST", data={"name": "Stage 1 unrelated key"})
        product = api(USER + "/products/", method="POST", token=token, expected=201,
                      data={"name": "Stage 1 product", "category_name": "Test", "price": 1000,
                            "initial_stock": 0, "status": "AKTIF"})["product"]
        upload(USER + "/products/" + product["id"] + "/thumbnail", token,
               "thumbnail", "stage1.png", "image/png", PNG)
        document = upload(DOCUMENT + "/documents/upload", token, "file", "stage1.txt",
                          "text/plain", TEXT, {"category": "GENERAL_DOCUMENT"}, expected=201)["document"]
        assert document["bucket_name"] == "test-documents", "Custom bucket name was ignored"
        state = {"product_id": product["id"], "document_id": document["id"],
                 "key_fingerprint": key_fingerprint(), "sentinel_key": sentinel["accessKeyId"]}
        STATE.write_text(json.dumps(state))
    elif sys.argv[1] == "verify":
        state = json.loads(STATE.read_text())
        assert key_fingerprint() == state["key_fingerprint"], "Bootstrap rotated the application key"
        assert any(key["id"] == state["sentinel_key"] for key in garage("ListKeys")), "Bootstrap removed an unrelated key"
        print("PASS: application credentials and unrelated keys survived", flush=True)
    else:
        raise ValueError("Use prepare or verify")
    verify_uploads(state, token)


if __name__ == "__main__":
    main()
