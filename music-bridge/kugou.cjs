const crypto = require("node:crypto");
const { createRequire } = require("node:module");
const kg = createRequire(require.resolve("kugoumusicapi/package.json"));
const axios = kg("axios");
// The adapter only uses HTTPS KuGou endpoints. No general proxy or challenge bypass.
axios.interceptors.request.use((config) => {
  const u = new URL(config.url, config.baseURL);
  if (u.hostname.endsWith(".kugou.com")) {
    if (u.protocol !== "https:") throw Error("https_required");
    config.timeout = 3000;
    config.maxRedirects = 0;
    config.proxy = false;
  }
  return config;
});
const { createRequest } = kg("./util/request");
const { calculateMid } = kg("./util/util");
const modules = {
  key: kg("./module/login_qr_key"),
  check: kg("./module/login_qr_check"),
  account: kg("./module/user_detail"),
  lists: kg("./module/user_playlist"),
  tracks: kg("./module/playlist_track_all_new"),
  publicTracks: kg("./module/playlist_track_all"),
  play: kg("./module/song_url"),
  device: kg("./module/register_dev"),
};
function cookieMerge(seed, values) {
  const result = { ...seed };
  for (const raw of values || []) {
    const [pair] = raw.split(";");
    const i = pair.indexOf("=");
    if (i > 0 && /^[A-Za-z0-9_]+$/.test(pair.slice(0, i)))
      result[pair.slice(0, i)] = pair.slice(i + 1);
  }
  return result;
}
function uid(v) {
  const s = String(v ?? "");
  return /^[1-9][0-9]{0,17}$/.test(s) ? s : "";
}
function pic(v) {
  return String(v || "")
    .replaceAll("{size}", "240")
    .replace(/^http:\/\//, "https://");
}
async function call(fn, data) {
  const result = await fn(data, createRequest);
  if (result.headers?.["ssa-code"] || result.body?.ssaCode)
    throw Error("verification_required");
  return result;
}
function normalizeList(p, own) {
  const local = p.listid ?? p.list_id;
  const id =
    local !== undefined
      ? String(local)
      : p.global_collection_id
        ? "g_" + p.global_collection_id
        : "";
  return {
    id,
    name: String(p.listname || p.name || p.special_name || "未命名歌单").slice(
      0,
      150,
    ),
    cover: pic(p.pic || p.imgurl || p.special_cover || p.cover),
    track_count:
      Number(p.count ?? p.song_count ?? p.file_count ?? p.total ?? 0) || 0,
    created: String(p.list_create_userid ?? p.userid ?? "") === own,
  };
}
function normalizeSong(p) {
  const hash = String(
    p.hash || p.filehash || p.audio_info?.hash || "",
  ).toUpperCase();
  if (!/^[A-F0-9]{32}$/.test(hash)) return null;
  const album = String(p.album_id || p.album_info?.album_id || 0),
    aid = String(
      p.album_audio_id || p.mixsongid || p.album_info?.album_audio_id || 0,
    );
  const filename = String(p.filename || p.name || p.songname || "未命名歌曲");
  return {
    id:
      hash +
      "_" +
      (/^\d+$/.test(album) ? album : "0") +
      "_" +
      (/^\d+$/.test(aid) ? aid : "0"),
    name: String(
      p.songname ||
        p.song_name ||
        filename.split(" - ").slice(1).join(" - ") ||
        filename,
    ).slice(0, 150),
    artist: String(
      p.singername || p.singer_name || filename.split(" - ")[0] || "",
    ).slice(0, 150),
    album: String(p.album_name || p.album_info?.album_name || ""),
    cover: pic(p.cover || p.imgurl || p.album_info?.sizable_cover),
    duration: Number(p.timelength || p.time_length)
      ? Number(p.timelength || p.time_length) / 1000
      : Number(p.duration || p.time_len || 0),
    fee: 0,
  };
}
function normalizeAudio(body) {
  const d = body.data && typeof body.data === "object" ? body.data : body;
  const raw = Array.isArray(d.url) ? d.url[0] : d.url;
  const flag = (v) => v === true || v === 1 || v === "1";
  const range =
    d.free_part_info || (typeof d.free_part === "object" ? d.free_part : null);
  const trial =
    flag(d.is_free_part) ||
    flag(d.IsFreePart) ||
    flag(d.free_part) ||
    (range && Number(range.end) > Number(range.start));
  const denied = {
    status: "unavailable",
    url: "",
    message:
      "酷狗暂未返回可确认的完整音源，可能受账号权限、地区、试听或验证限制。",
  };
  if (body.status !== 1 || typeof raw !== "string" || !raw || trial)
    return denied;
  return {
    status: "playable",
    url: raw,
    message: "酷狗 · 标准音质",
    expires_in: 300,
    trial_start: 0,
    trial_end: 0,
  };
}
async function handleKugou(path, body) {
  if (path === "/qr/start") {
    const guid = crypto.randomBytes(16).toString("hex");
    let cookie = {
      KUGOU_API_GUID: guid,
      KUGOU_API_MID: calculateMid(guid),
      KUGOU_API_DEV: crypto.randomBytes(16).toString("hex").toUpperCase(),
      KUGOU_API_MAC: "02:00:00:00:00:00",
      dfid: "-",
    };
    const dev = await call(modules.device, { cookie });
    cookie = cookieMerge(cookie, dev.cookie);
    const response = await call(modules.key, { cookie });
    const data = response.body?.data;
    if (!data?.qrcode || !data?.qrcode_img) throw Error("invalid_qr");
    return {
      status: 200,
      data: {
        key: String(data.qrcode),
        image: data.qrcode_img,
        cookie: cookieMerge(cookie, response.cookie),
      },
    };
  }
  const cookie = body.cookie || {};
  if (path === "/qr/check") {
    if (typeof body.key !== "string" || body.key.length > 256)
      return { status: 400, data: { error: "invalid_key" } };
    const response = await call(modules.check, { key: body.key, cookie });
    const state = Number(response.body?.data?.status),
      codes = { 0: 800, 1: 801, 2: 802, 4: 803 };
    if (!codes[state]) throw Error("invalid_qr_state");
    const data = { code: codes[state] };
    if (state === 4) {
      data.cookie = cookieMerge(cookie, response.cookie);
      const own = response.body.data;
      data.cookie.token = own.token;
      data.cookie.userid = String(own.userid);
      if (!data.cookie.token || !uid(data.cookie.userid))
        throw Error("invalid_login");
    }
    return { status: 200, data };
  }
  if (!cookie.token || !uid(cookie.userid))
    return { status: 401, data: { error: "session_expired" } };
  // Validate credentials rather than trusting a user ID supplied by a browser.
  const response = await call(modules.account, { cookie });
  const own = response.body?.data;
  if (!own || !(response.body.status === 1 || response.body.code === 200))
    throw Error("account_unavailable");
  if (own.userid && String(own.userid) !== String(cookie.userid))
    return { status: 401, data: { error: "session_expired" } };
  const ownID = String(cookie.userid);
  if (path === "/account")
    return {
      status: 200,
      data: {
        uid: ownID,
        nickname: String(own.nickname || own.nick_name || "酷狗用户").slice(
          0,
          100,
        ),
        avatar: pic(own.pic || own.avatar || own.user_pic),
      },
    };
  const offset = Number(body.offset || 0);
  if (!Number.isInteger(offset) || offset < 0 || offset > 20000)
    return { status: 400, data: { error: "invalid_offset" } };
  if (path === "/playlists") {
    const result = await call(modules.lists, {
      cookie,
      page: Math.floor(offset / 20) + 1,
      pagesize: 20,
    });
    const d = result.body?.data;
    const list = d?.list || d?.info;
    if (!Array.isArray(list)) throw Error("playlist_unavailable");
    return {
      status: 200,
      data: {
        uid: ownID,
        offset,
        more:
          Number(d.total ?? d.count) > offset + list.length ||
          list.length === 20,
        items: list.map((p) => normalizeList(p, ownID)).filter((p) => p.id),
      },
    };
  }
  if (path === "/tracks") {
    const id = String(body.playlist_id || "");
    if (!/^(?:\d{1,18}|g_[A-Za-z0-9]{1,64})$/.test(id))
      return { status: 400, data: { error: "invalid_playlist" } };
    const isPublic = id.startsWith("g_");
    const result = await call(
      isPublic ? modules.publicTracks : modules.tracks,
      {
        cookie,
        ...(isPublic ? { id: id.slice(2) } : { listid: id }),
        page: Math.floor(offset / 30) + 1,
        pagesize: 30,
      },
    );
    const d = result.body?.data;
    const info = d?.info || d?.list;
    if (!Array.isArray(info)) throw Error("tracks_unavailable");
    const total = Number(d.count ?? d.total ?? info.length);
    return {
      status: 200,
      data: {
        uid: ownID,
        playlist_id: id,
        name: String(d.listname || d.name || "酷狗歌单"),
        offset,
        total,
        more: offset + info.length < total,
        items: info.map(normalizeSong).filter(Boolean),
      },
    };
  }
  if (path === "/playback") {
    const id = String(body.track_id || "");
    if (!/^[A-Fa-f0-9]{32}(?:_\d{1,18}_\d{1,18})?$/.test(id))
      return { status: 400, data: { error: "invalid_track" } };
    const [hash, album_id = "0", album_audio_id = "0"] = id.split("_");
    const result = await call(modules.play, {
      cookie,
      hash,
      album_id,
      album_audio_id,
      quality: 128,
      free_part: false,
    });
    return {
      status: 200,
      data: { uid: ownID, track_id: id, ...normalizeAudio(result.body) },
    };
  }
  return { status: 404, data: { error: "not_found" } };
}
module.exports = { handleKugou, normalizeList, normalizeSong, normalizeAudio };
