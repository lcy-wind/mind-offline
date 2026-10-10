// This is a private allowlisted adapter, not the upstream project's public API server.
const http = require("node:http");
const crypto = require("node:crypto");
const fs = require("node:fs");
const os = require("node:os");
const path = require("node:path");
// Upstream may log error objects containing cookies. Never emit upstream objects.
for (const level of ["log", "info", "warn", "error", "debug"])
  console[level] = () => {};
process.env.ENABLE_GENERAL_UNBLOCK = "false";
process.env.ENABLE_RANDOM_CN_IP = "false";
const anon = path.join(os.tmpdir(), "anonymous_token");
if (!fs.existsSync(anon)) fs.writeFileSync(anon, "", { mode: 0o600 });
const sdk = "@neteasecloudmusicapienhanced/api";
// Load only approved modules; never main.js/server.js or unlock modules.
const upstream = require(sdk + "/util/request");
const qrKey = require(sdk + "/module/login_qr_key");
const qrCreate = require(sdk + "/module/login_qr_create");
const qrCheck = require(sdk + "/module/login_qr_check");
const account = require(sdk + "/module/user_account");
const playlists = require(sdk + "/module/user_playlist");
const playlistDetail = require(sdk + "/module/playlist_detail");
const songDetail = require(sdk + "/module/song_detail");
const createOption = require(sdk + "/util/option");
const { normalizePlayback, normalizeTrack } = require("./playback.cjs");
const { handleKugou } = require("./kugou.cjs");
const secret = process.env.NCM_BRIDGE_TOKEN;
if (!secret || secret.length < 32) throw Error("NCM_BRIDGE_TOKEN is required");
const secretHash = crypto
  .createHash("sha256")
  .update("Bearer " + secret)
  .digest();
function send(res, status, data) {
  res.writeHead(status, {
    "Content-Type": "application/json",
    "Cache-Control": "no-store",
  });
  res.end(JSON.stringify(data));
}
function mergeCookies(context, values) {
  const out = { ...context };
  for (const raw of values || []) {
    const pair = raw.split(";", 1)[0],
      i = pair.indexOf("=");
    if (i > 0) {
      const name = pair.slice(0, i).trim();
      if (/^[A-Za-z0-9_]+$/.test(name)) out[name] = pair.slice(i + 1);
    }
  }
  return out;
}
function uid(value) {
  const s = String(value ?? "");
  return /^[1-9][0-9]{0,17}$/.test(s) ? s : "";
}
const server = http.createServer(async (req, res) => {
  if (req.method === "GET" && req.url === "/health") {
    send(res, 200, { status: "ok" });
    return;
  }
  const actual = crypto
    .createHash("sha256")
    .update(req.headers.authorization || "")
    .digest();
  if (!crypto.timingSafeEqual(secretHash, actual)) {
    send(res, 401, { error: "unauthorized" });
    return;
  }
  if (
    req.method !== "POST" ||
    ![
      "/qr/start",
      "/qr/check",
      "/account",
      "/playlists",
      "/tracks",
      "/playback",
    ].includes(req.url.startsWith("/kugou/") ? req.url.slice(6) : req.url)
  ) {
    send(res, 404, { error: "not_found" });
    return;
  }
  let raw = "";
  try {
    for await (const chunk of req) {
      raw += chunk;
      if (Buffer.byteLength(raw) > 65536) {
        send(res, 413, { error: "too_large" });
        return;
      }
    }
    let body;
    try {
      body = JSON.parse(raw);
    } catch {
      send(res, 400, { error: "invalid_json" });
      return;
    }
    if (req.url.startsWith("/kugou/")) {
      const result = await handleKugou(req.url.slice(6), body);
      send(res, result.status, result.data);
      return;
    }
    const options = {
      cookie: body.cookie || {},
      timeout: req.url === "/tracks" ? 2500 : 4000,
    };
    if (req.url === "/qr/start") {
      const seed = {
        os: "pc",
        deviceId: crypto.randomBytes(16).toString("hex"),
      };
      const result = await qrKey({ cookie: seed, timeout: 4000 }, upstream);
      const key = result.body?.data?.unikey;
      if (typeof key !== "string" || !key || key.length > 256)
        throw Error("upstream");
      const qr = await qrCreate({ key, qrimg: true, platform: "pc" });
      send(res, 200, {
        key,
        cookie: mergeCookies(seed, result.cookie),
        image: qr.body.data.qrimg,
      });
      return;
    }
    if (req.url === "/qr/check") {
      if (typeof body.key !== "string" || body.key.length > 256) {
        send(res, 400, { error: "invalid_key" });
        return;
      }
      const result = await qrCheck({ ...options, key: body.key }, upstream);
      const code = Number(result.body?.code);
      if (![800, 801, 802, 803].includes(code)) throw Error("upstream");
      const data = { code };
      if (code === 803) {
        data.cookie = mergeCookies(body.cookie, result.cookie);
        if (!data.cookie.MUSIC_U) throw Error("upstream");
      }
      send(res, 200, data);
      return;
    }
    const own = await account(options, upstream);
    const profile = own.body?.profile;
    if (!profile || !uid(profile.userId)) {
      send(res, 401, { error: "session_expired" });
      return;
    }
    if (req.url === "/account") {
      send(res, 200, {
        uid: uid(profile.userId),
        nickname: String(profile.nickname || "网易云用户").slice(0, 100),
        avatar: String(profile.avatarUrl || ""),
      });
      return;
    }
    if (req.url === "/playback") {
      if (!uid(body.track_id)) {
        send(res, 400, { error: "invalid_track" });
        return;
      }
      const result = await upstream(
        "/api/song/enhance/player/url/v1",
        {
          ids: "[" + body.track_id + "]",
          level: "standard",
          encodeType: "mp3",
        },
        createOption(options, "eapi"),
      );
      const entry = result.body?.data?.find((t) => uid(t.id) === body.track_id);
      send(res, 200, {
        uid: uid(profile.userId),
        track_id: body.track_id,
        ...normalizePlayback(entry),
      });
      return;
    }
    if (req.url === "/tracks") {
      const offset = Number(body.offset || 0);
      if (
        !uid(body.playlist_id) ||
        !Number.isInteger(offset) ||
        offset < 0 ||
        offset > 20000
      ) {
        send(res, 400, { error: "invalid_playlist" });
        return;
      }
      const result = await playlistDetail(
        { ...options, id: body.playlist_id },
        upstream,
      );
      const list = result.body?.playlist;
      if (!list || !Array.isArray(list.trackIds)) {
        send(res, 403, { error: "playlist_unavailable" });
        return;
      }
      const ids = list.trackIds
        .slice(offset, offset + 30)
        .map((t) => uid(t.id))
        .filter(Boolean);
      let songs = [];
      if (ids.length) {
        const details = await songDetail(
          { ...options, ids: ids.join(",") },
          upstream,
        );
        if (details.body?.code !== 200 || !Array.isArray(details.body.songs))
          throw Error("upstream");
        songs = ids
          .map((id) => details.body.songs.find((t) => uid(t.id) === id))
          .filter(Boolean)
          .map(normalizeTrack);
      }
      send(res, 200, {
        uid: uid(profile.userId),
        playlist_id: body.playlist_id,
        name: String(list.name || "歌单").slice(0, 150),
        items: songs,
        total: list.trackIds.length,
        offset,
        more: offset + 30 < list.trackIds.length,
      });
      return;
    }
    // UID is derived from the credential; callers cannot request another user's playlists.
    const offset = Number(body.offset || 0);
    if (!Number.isInteger(offset) || offset < 0 || offset > 20000) {
      send(res, 400, { error: "invalid_offset" });
      return;
    }
    const result = await playlists(
      { ...options, uid: profile.userId, limit: 20, offset },
      upstream,
    );
    if (result.body?.code !== 200 || !Array.isArray(result.body.playlist))
      throw Error("upstream");
    send(res, 200, {
      uid: uid(profile.userId),
      more: Boolean(result.body.more),
      items: result.body.playlist
        .filter((p) => uid(p.id))
        .map((p) => ({
          id: uid(p.id),
          name: String(p.name || "未命名歌单").slice(0, 150),
          cover: String(p.coverImgUrl || ""),
          track_count: Number(p.trackCount) || 0,
          created: uid(p.creator?.userId) === uid(profile.userId),
        })),
    });
  } catch {
    send(res, 502, { error: "upstream_unavailable" });
  }
});
server.requestTimeout = 15000;
server.headersTimeout = 10000;
server.listen(Number(process.env.NCM_BRIDGE_PORT || 18085), "127.0.0.1", () =>
  process.stdout.write(
    "music bridge listening on loopback:" + server.address().port + "\n",
  ),
);
process.on("SIGTERM", () => server.close(() => process.exit(0)));
