const test = require("node:test");
const assert = require("node:assert/strict");
const { spawn } = require("node:child_process");
const { once } = require("node:events");
const { mkdtemp, rm } = require("node:fs/promises");
const { tmpdir } = require("node:os");
const { join } = require("node:path");
test("private adapter authenticates and exposes only approved routes", async () => {
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
      "/kugou/login/cellphone",
      "/kugou/song/url",
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

const { normalizePlayback } = require("./playback.cjs");
test("playback distinguishes full, trial and unavailable without substituting audio", () => {
  assert.equal(
    normalizePlayback({ code: 404, url: null }).status,
    "unavailable",
  );
  assert.equal(
    normalizePlayback({
      code: 200,
      url: "https://m7.music.126.net/test.mp3",
      freeTrialInfo: null,
    }).status,
    "playable",
  );
  const preview = normalizePlayback({
    code: 200,
    url: "https://m7.music.126.net/preview.mp3",
    freeTrialInfo: { start: 60, end: 90 },
  });
  assert.equal(preview.status, "trial");
  assert.equal(preview.trial_start, 60);
  assert.equal(preview.trial_end, 90);
  assert.equal(
    normalizePlayback({
      code: 200,
      url: "https://m7.music.126.net/test.mp3",
      freeTrialInfo: {},
    }).status,
    "unavailable",
  );
});
const { normalizeList, normalizeSong, normalizeAudio } = require("./kugou.cjs");
test("KuGou mappings whitelist account data and reject uncertain preview audio", () => {
  const l = normalizeList(
    {
      listid: 3,
      listname: "我的歌单",
      pic: "http://imge.kugou.com/{size}/test.jpg",
      count: 2,
      list_create_userid: 123,
      token: "never-expose",
    },
    "123",
  );
  assert.equal(l.id, "3");
  assert.equal(l.created, true);
  assert.equal(l.cover, "https://imge.kugou.com/240/test.jpg");
  assert.equal(l.token, undefined);
  const s = normalizeSong({
    hash: "a".repeat(32),
    filename: "歌手 - 歌名",
    album_id: 1,
    album_audio_id: 2,
    timelength: 90000,
  });
  assert.equal(s.id, "A".repeat(32) + "_1_2");
  assert.equal(s.duration, 90);
  assert.equal(s.name, "歌名");
  assert.equal(s.artist, "歌手");
  assert.equal(normalizeSong({ hash: "bad" }), null);
  assert.equal(
    normalizeAudio({
      status: 1,
      url: ["https://fs.open.kugou.com/test.mp3"],
      is_free_part: 0,
    }).status,
    "playable",
  );
  assert.equal(
    normalizeAudio({
      status: 1,
      url: ["https://fs.open.kugou.com/test.mp3"],
      is_free_part: "0",
      free_part_info: { start: 0, end: 0 },
    }).status,
    "playable",
  );
  assert.equal(
    normalizeAudio({
      status: 1,
      url: ["https://fs.open.kugou.com/test.mp3"],
      is_free_part: 1,
    }).status,
    "unavailable",
  );
  assert.equal(normalizeAudio({ status: 1, url: [] }).status, "unavailable");
});
