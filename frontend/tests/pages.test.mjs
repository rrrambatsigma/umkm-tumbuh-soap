import assert from "node:assert/strict";
import { after, before, test } from "node:test";
import { createServer as createViteServer } from "vite";

let vite, auth;

before(async () => {
  vite = await createViteServer({
    configFile: false,
    server: { middlewareMode: true, hmr: false },
  });
  auth = await vite.ssrLoadModule("/src/features/auth/api.ts");
});

after(async () => {
  await vite?.close();
});

test("registration upload returns the document ID and sends the selected file", async (t) => {
  const file = new File(["test document"], "legal.txt", { type: "text/plain" });
  t.mock.method(globalThis, "fetch", async (url, options) => {
    assert.equal(url, "/api/v1/documents/upload");
    assert.equal(options.headers.Authorization, "Bearer test-token");
    assert.equal(options.body.get("category"), "LEGALITAS");
    assert.equal(await options.body.get("file").text(), "test document");
    return Response.json({ document: { id: "doc-test" } });
  });
  const result = await auth.uploadRegistrationDocument(file, "LEGALITAS");
  assert.equal(result.document.id, "doc-test");
});

test("registration upload rejects a success response without a document ID", async (t) => {
  t.mock.method(globalThis, "fetch", async () => Response.json({ message: "Uploaded" }));
  await assert.rejects(auth.uploadRegistrationDocument(new File(["test"], "legal.txt"), "LEGALITAS"),
    /tidak memuat ID dokumen/);
});

test("registration upload preserves a backend validation error", async (t) => {
  t.mock.method(globalThis, "fetch", async () => Response.json({ error: "Jenis berkas tidak didukung" }, { status: 400 }));
  await assert.rejects(auth.uploadRegistrationDocument(new File(["test"], "legal.txt"), "LEGALITAS"),
    /Jenis berkas tidak didukung/);
});
