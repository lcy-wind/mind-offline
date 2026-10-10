export interface MusicTrack {
  id: string;
  name: string;
  artist: string;
  album: string;
  cover: string;
  duration: number;
  fee: number;
}
export interface Playback {
  uid: string;
  track_id: string;
  status: "playable" | "trial" | "unavailable";
  url: string;
  message: string;
  expires_in: number;
  trial_start: number;
  trial_end: number;
}
export interface AudioEvents {
  meta: (duration: number) => void;
  time: (seconds: number) => void;
  play: () => void;
  pause: () => void;
  ended: () => void;
  error: (message: string) => void;
}
export interface AudioDriver {
  load: (url: string) => void;
  play: () => Promise<void>;
  pause: () => void;
  seek: (seconds: number) => void;
  destroy: () => void;
}
export interface PlayerState {
  track: MusicTrack | null;
  queue: MusicTrack[];
  index: number;
  playlistId: string;
  status: string;
  message: string;
  current: number;
  duration: number;
  trial: boolean;
  bindingUid: string;
}
export const emptyPlayerState = (): PlayerState => ({
  track: null,
  queue: [],
  index: -1,
  playlistId: "",
  status: "idle",
  message: "",
  current: 0,
  duration: 0,
  trial: false,
  bindingUid: "",
});
export function createPlayer(
  state: PlayerState,
  fetchPlayback: (id: string) => Promise<Playback>,
  makeAudio: (events: AudioEvents) => AudioDriver,
  getToken: () => string,
) {
  let owner = "",
    session = "",
    revision = 0,
    audio: AudioDriver | null = null,
    reply: Playback | null = null,
    mediaDuration = 0,
    expiresAt = 0,
    wantPlay = false,
    finished = false,
    initialSeekDone = false;
  function dispose() {
    const old = audio;
    audio = null;
    old?.destroy();
  }
  function reset() {
    revision++;
    wantPlay = false;
    dispose();
    reply = null;
    Object.assign(state, emptyPlayerState());
  }
  function setOwner(id: string) {
    const current = getToken();
    if (owner !== id || session !== current) {
      reset();
      owner = id;
      session = current;
    }
  }
  function valid() {
    if (!owner || !session || getToken() !== session) {
      reset();
      return false;
    }
    return true;
  }
  function bounds() {
    if (!reply || !state.trial)
      return { start: 0, end: mediaDuration || state.track?.duration || 0 };
    const length = reply.trial_end - reply.trial_start;
    if (mediaDuration > 0 && mediaDuration <= length + 1)
      return { start: 0, end: Math.min(mediaDuration, length) };
    return {
      start: reply.trial_start,
      end:
        mediaDuration > 0
          ? Math.min(reply.trial_end, mediaDuration)
          : reply.trial_end,
    };
  }
  function finish() {
    if (finished) return;
    finished = true;
    wantPlay = false;
    audio?.pause();
    state.status = "ended";
    if (state.index + 1 < state.queue.length)
      void select(state.queue[state.index + 1], state.queue, state.playlistId);
    else state.message = "已播放完当前队列";
  }
  async function resume() {
    if (!audio || !valid()) return;
    const driver = audio;
    wantPlay = true;
    try {
      await driver.play();
    } catch (e) {
      if (driver !== audio || !wantPlay) return;
      state.status = "blocked";
      state.message = "浏览器尚未开始播放，请再点一次播放按钮";
    }
  }
  async function select(
    track: MusicTrack,
    queue: MusicTrack[],
    playlistId: string,
  ) {
    if (!valid()) return;
    const rev = ++revision;
    dispose();
    reply = null;
    mediaDuration = 0;
    finished = false;
    initialSeekDone = false;
    wantPlay = true;
    state.track = track;
    state.queue = [...queue];
    state.index = queue.findIndex((t) => t.id === track.id);
    state.playlistId = playlistId;
    state.status = "loading";
    state.message = "正在获取音源…";
    state.current = 0;
    state.duration = track.duration;
    state.trial = false;
    state.bindingUid = "";
    try {
      const data = await fetchPlayback(track.id);
      if (rev !== revision || !valid()) return;
      if (data.status === "unavailable" || !data.url) {
        state.status = "unavailable";
        state.message = data.message || "这首歌曲暂时无法播放";
        wantPlay = false;
        return;
      }
      reply = data;
      state.trial = data.status === "trial";
      state.bindingUid = data.uid;
      expiresAt = Date.now() + Math.max(0, data.expires_in) * 1000;
      if (state.trial) state.duration = data.trial_end - data.trial_start;
      let driver: AudioDriver;
      const active = () => driver === audio && valid();
      driver = makeAudio({
        meta: (duration) => {
          if (!active() || duration <= 0) return;
          mediaDuration = duration;
          const b = bounds();
          state.duration = Math.max(0, b.end - b.start);
          if (state.trial && b.start > 0 && !initialSeekDone)
            driver.seek(b.start);
          initialSeekDone = true;
        },
        time: (seconds) => {
          if (!active()) return;
          const b = bounds();
          state.current = Math.max(
            0,
            Math.min(seconds - b.start, state.duration),
          );
          if (state.trial && b.end > 0 && seconds >= b.end) finish();
        },
        play: () => {
          if (!active()) return;
          state.status = "playing";
          state.message = state.trial ? "试听片段" : "标准音质";
        },
        pause: () => {
          if (!active() || finished) return;
          state.status = "paused";
        },
        ended: () => {
          if (active()) finish();
        },
        error: (message) => {
          if (!active()) return;
          wantPlay = false;
          state.status = "error";
          state.message = message || "音源暂不可用，请重试或前往网易云";
        },
      });
      audio = driver;
      driver.load(data.url);
      await resume();
    } catch (e) {
      if (rev !== revision || !valid()) return;
      state.status = "error";
      state.message = (e as Error).message;
      wantPlay = false;
    }
  }
  function pause() {
    wantPlay = false;
    if (state.status === "loading") {
      revision++;
      state.status = "paused";
      state.message = "已暂停加载";
      return;
    }
    audio?.pause();
    if (state.track) state.status = "paused";
  }
  function toggle() {
    if (!valid() || !state.track) return;
    if (state.status === "playing" || state.status === "loading") {
      pause();
      return;
    }
    if (
      audio &&
      Date.now() < expiresAt &&
      !["ended", "error", "unavailable"].includes(state.status)
    )
      void resume();
    else void select(state.track, state.queue, state.playlistId);
  }
  function move(delta: number) {
    if (!valid()) return;
    const index = state.index + delta;
    if (index >= 0 && index < state.queue.length)
      void select(state.queue[index], state.queue, state.playlistId);
  }
  function seek(seconds: number) {
    if (!valid() || !audio || !Number.isFinite(seconds)) return;
    const b = bounds();
    audio.seek(
      b.start +
        Math.max(0, Math.min(seconds, Math.max(0, b.end - b.start - 0.1))),
    );
  }
  function extendQueue(playlistId: string, tracks: MusicTrack[]) {
    if (state.playlistId === playlistId) {
      state.queue = [...tracks];
      state.index = tracks.findIndex((t) => t.id === state.track?.id);
    }
  }
  return {
    state,
    setOwner,
    reset,
    select,
    pause,
    toggle,
    move,
    seek,
    extendQueue,
  };
}
