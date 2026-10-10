function normalizePlayback(entry) {
  if (!entry || Number(entry.code) !== 200 || !entry.url)
    return {
      status: "unavailable",
      url: "",
      message: "当前歌曲无法在食堂播放，可能受账号权限、地区或版权限制。",
    };
  const trial = entry.freeTrialInfo && typeof entry.freeTrialInfo === "object";
  const start = Number(entry.freeTrialInfo?.start),
    end = Number(entry.freeTrialInfo?.end);
  // Only honor an explicit, plausible provider trial range. Unknown trial ranges fail closed.
  if (
    trial &&
    (!Number.isFinite(start) ||
      !Number.isFinite(end) ||
      start < 0 ||
      end <= start ||
      end - start > 600)
  )
    return {
      status: "unavailable",
      url: "",
      message:
        "平台返回了试听权限，但未提供可确认的试听范围，请前往网易云收听。",
    };
  return {
    status: trial ? "trial" : "playable",
    url: entry.url,
    trial_start: trial ? start : 0,
    trial_end: trial ? end : 0,
    expires_in: Math.max(30, Math.min(1800, Number(entry.expi) || 300)),
    message: trial ? "试听片段" : "标准音质",
  };
}
function normalizeTrack(s) {
  return {
    id: String(s.id),
    name: String(s.name || "未命名歌曲").slice(0, 150),
    artist: (s.ar || s.artists || [])
      .map((a) => a.name)
      .filter(Boolean)
      .join(" / ")
      .slice(0, 200),
    album: String(s.al?.name || s.album?.name || "").slice(0, 150),
    cover: String(s.al?.picUrl || s.album?.picUrl || ""),
    duration: Math.max(0, Number(s.dt || s.duration || 0) / 1000),
    fee: Number(s.fee) || 0,
  };
}
module.exports = { normalizePlayback, normalizeTrack };
