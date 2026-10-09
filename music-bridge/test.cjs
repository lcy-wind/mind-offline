const test = require("node:test");
const assert = require("node:assert/strict");
const { spawn } = require("node:child_process");
const { once } = require("node:events");
const { mkdtemp, rm } = require("node:fs/promises");
const { tmpdir } = require("node:os");
const { join } = require("node:path");
test("private adapter authenticates and exposes only the four approved routes", async () => {
  const dir = await mkdtemp(join(tmpdir(), "mind-offline-music-test-"));
  const secret = "test-bridge-secret-that-is-never-used-in-production";
  const child = spawn(process.execPath, ["server.cjs"], {
    env: {
      ...process.env,
      NCM_BRIDGE_TOKEN: secret,
      NCM_BRIDGE_PORT: "0",
      TMPDIR: dir,
    },
    stdio: ["ignore", "pipe", "pipe"],
  });
  try {
    const endpoint = await new Promise((resolve, reject) => {
      const timeout = setTimeout(
        () => reject(Error("startup timed out")),
        15000,
      );
      child.stdout.on("data", (chunk) => {
        const match = chunk.toString().match(/loopback:(\d+)/);
        if (match) {
          clearTimeout(timeout);
          resolve("http://127.0.0.1:" + match[1]);
        }
      });
      child.on("exit", (code) => {
        clearTimeout(timeout);
        reject(Error("unexpected exit " + code));
      });
    });
    assert.equal((await fetch(endpoint + "/health")).status, 200);
    assert.equal(
      (await fetch(endpoint + "/qr/start", { method: "POST", body: "{}" }))
        .status,
      401,
    );
    for (const path of [
      "/login/cellphone",
      "/song/url",
      "/logout",
      "/admin",
      "/user/playlist",
    ]) {
      const res = await fetch(endpoint + path, {
        method: "POST",
        body: "{}",
        headers: { Authorization: "Bearer " + secret },
      });
      assert.equal(res.status, 404);
      assert.equal((await res.json()).error, "not_found");
    }
    const invalid = await fetch(endpoint + "/qr/start", {
      method: "POST",
      body: "bad json",
      headers: { Authorization: "Bearer " + secret },
    });
    assert.equal(invalid.status, 400);
  } finally {
    child.kill("SIGTERM");
    await once(child, "exit");
    await rm(dir, { recursive: true, force: true });
  }
});
