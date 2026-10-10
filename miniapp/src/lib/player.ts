import { reactive } from "vue";
import { request, currentToken } from "./api";
import {
  createPlayer,
  emptyPlayerState,
  type AudioDriver,
  type AudioEvents,
  type Playback,
} from "./player-core";
export type { MusicTrack, MusicProvider } from "./player-core";
export const playerState = reactive(emptyPlayerState());
function audioDriver(events: AudioEvents): AudioDriver {
  // #ifdef H5
  const a = new Audio();
  a.preload = "metadata";
  a.volume = 0.65;
  a.addEventListener("loadedmetadata", () =>
    events.meta(Number.isFinite(a.duration) ? a.duration : 0),
  );
  a.addEventListener("durationchange", () => {
    if (Number.isFinite(a.duration)) events.meta(a.duration);
  });
  a.addEventListener("timeupdate", () => events.time(a.currentTime));
  a.addEventListener("play", events.play);
  a.addEventListener("pause", events.pause);
  a.addEventListener("ended", events.ended);
  a.addEventListener("error", () =>
    events.error(
      "音源加载失败，可能已过期或暂不可用。点击播放可重试，也可前往网易云收听。",
    ),
  );
  return {
    load: (url) => {
      a.src = url;
    },
    play: () => a.play(),
    pause: () => a.pause(),
    seek: (s) => {
      a.currentTime = s;
    },
    destroy: () => {
      a.pause();
      a.removeAttribute("src");
      a.load();
    },
  };
  // #endif
  // #ifndef H5
  const native = uni.createInnerAudioContext();
  native.autoplay = false;
  native.volume = 0.65;
  native.onCanplay(() => events.meta(native.duration));
  native.onTimeUpdate(() => {
    events.meta(native.duration);
    events.time(native.currentTime);
  });
  native.onPlay(events.play);
  native.onPause(events.pause);
  native.onEnded(events.ended);
  native.onError(() =>
    events.error("当前音源无法播放，请重试或前往网易云收听。"),
  );
  return {
    load: (url) => {
      native.src = url;
    },
    play: async () => {
      native.play();
    },
    pause: () => native.pause(),
    seek: (s) => native.seek(s),
    destroy: () => native.destroy(),
  };
  // #endif
}
export const player = createPlayer(
  playerState,
  (id, provider) =>
    request<Playback>("/music/" + provider + "/playback", "POST", {
      track_id: id,
    }),
  audioDriver,
  currentToken,
);
let checkedAt = 0;
export async function checkPlayerBinding() {
  if (!playerState.track || Date.now() - checkedAt < 15000) return;
  checkedAt = Date.now();
  const session = currentToken();
  const provider = playerState.provider;
  try {
    const binding = await request<{
      bound: boolean;
      expired?: boolean;
      uid?: string;
    }>("/music/" + provider);
    if (
      currentToken() === session &&
      playerState.provider === provider &&
      (!binding.bound ||
        binding.expired ||
        (playerState.bindingUid && binding.uid !== playerState.bindingUid))
    )
      player.reset();
  } catch {
    /* A temporary status request failure must not disturb an otherwise valid stream. */
  }
}
export function formatTime(seconds: number) {
  if (!Number.isFinite(seconds) || seconds < 0) return "0:00";
  return (
    Math.floor(seconds / 60) +
    ":" +
    String(Math.floor(seconds % 60)).padStart(2, "0")
  );
}
