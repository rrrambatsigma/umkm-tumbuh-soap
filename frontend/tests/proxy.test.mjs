import assert from "node:assert/strict";
import { once } from "node:events";
import { createServer } from "node:http";
import { after, before, test } from "node:test";
import { createServer as createViteServer, resolveConfig } from "vite";

const servicePorts = {
  auth: "AUTH_SERVICE_PORT",
  user: "USER_SERVICE_PORT",
  partnership: "PARTNERSHIP_SERVICE_PORT",
  document: "DOCUMENT_SERVICE_PORT",
};
const servers = [];
const originalEnv = new Map();
let vite;
let baseUrl;

function setEnv(key, value) {
  if (!originalEnv.has(key)) originalEnv.set(key, process.env[key]);
  process.env[key] = value;
}

before(async () => {
  // Port acak mencegah pengujian mengakses backend atau akun pengguna asli.
  for (const [service, variable] of Object.entries(servicePorts)) {
    const server = createServer(async (request, response) => {
      let body = "";
      for await (const chunk of request) body += chunk;
      response.setHeader("Content-Type", "application/json");
      response.end(JSON.stringify({
        service,
        path: request.url,
        method: request.method,
        authorization: request.headers.authorization,
        body,
      }));
    });
    servers.push(server);
    server.listen(0, "127.0.0.1");
    await once(server, "listening");
    setEnv(variable, String(server.address().port));
  }
  setEnv("VITE_USE_DEV_PROXY", "true");
  vite = await createViteServer({
    server: { host: "127.0.0.1", port: 0, hmr: false },
  });
  await vite.listen();
  baseUrl = `http://127.0.0.1:${vite.httpServer.address().port}`;
});

after(async () => {
  await vite?.close();
  for (const server of servers) {
    server.closeAllConnections();
    await new Promise((resolve) => server.close(resolve));
  }
  for (const [key, value] of originalEnv) {
    if (value === undefined) delete process.env[key];
    else process.env[key] = value;
  }
});

for (const service of Object.keys(servicePorts)) {
  test(`${service} proxy preserves the path and query on its configured port`, async () => {
    const response = await fetch(`${baseUrl}/backend/${service}/api/v1/probe?value=1`);
    assert.equal(response.status, 200);
    const result = await response.json();
    assert.equal(result.service, service);
    assert.equal(result.path, "/api/v1/probe?value=1");
  });
}

test("the actual login client sends its POST through Vite", async () => {
  const auth = await vite.ssrLoadModule("/src/features/auth/api.ts");
  const originalFetch = globalThis.fetch;
  // Node memerlukan origin eksplisit untuk URL relatif yang dipakai browser.
  globalThis.fetch = (input, options) => originalFetch(new URL(input, baseUrl), options);
  try {
    const credentials = { email: "proxy@stage1.test", password: "test-password" };
    const result = await auth.login(credentials);
    assert.equal(result.service, "auth");
    assert.equal(result.path, "/api/v1/auth/login");
    assert.equal(result.method, "POST");
    assert.deepEqual(JSON.parse(result.body), credentials);
  } finally {
    globalThis.fetch = originalFetch;
  }
});

test("existing relative document requests still reach the document service", async () => {
  const response = await fetch(`${baseUrl}/api/v1/documents/upload`, {
    method: "POST",
    body: "test-document",
  });
  const result = await response.json();
  assert.equal(result.service, "document");
  assert.equal(result.path, "/api/v1/documents/upload");
  assert.equal(result.body, "test-document");
});

test("production builds keep explicit API URLs even when the flag is enabled", async () => {
  const config = await resolveConfig({}, "build");
  assert.equal(config.define?.["import.meta.env.VITE_AUTH_API_BASE_URL"], undefined);
});

test("development can opt out of proxy URL overrides", async () => {
  setEnv("VITE_USE_DEV_PROXY", "false");
  const config = await resolveConfig({}, "serve");
  assert.equal(config.define?.["import.meta.env.VITE_AUTH_API_BASE_URL"], undefined);
});
