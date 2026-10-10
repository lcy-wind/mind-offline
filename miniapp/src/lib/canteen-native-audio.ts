export interface CanteenAudio {
  currentTime: number; readonly duration: number; readonly paused: boolean;
  readonly error: boolean; volume: number;
  load(): void; play(): Promise<void>; pause(): void;
  removeAttribute(name: string): void; destroy(): void;
}
type AudioEvent = "time" | "playing" | "pause" | "ended" | "error" | "waiting";
export function createCanteenAudio(url: () => string, notify: (event: AudioEvent) => void): CanteenAudio {
  let context: UniApp.InnerAudioContext | null = null;
  let paused = true, failed = false, volume = .65;
  function clear() { const old = context; context = null; old?.destroy(); paused = true; failed = false; }
  return {
    get currentTime() { return context?.currentTime || 0; },
    set currentTime(seconds: number) { context?.seek(seconds); },
    get duration() { return context?.duration || 0; },
    get paused() { return paused; }, get error() { return failed; },
    get volume() { return volume; }, set volume(value: number) { volume = value; if (context) context.volume = value; },
    load() {
      clear(); if (!url()) return;
      const audio = uni.createInnerAudioContext(); context = audio;
      const emit = (event: AudioEvent) => { if (context === audio) notify(event); };
      audio.autoplay = false; audio.volume = volume; audio.obeyMuteSwitch = false;
      audio.onCanplay(() => emit("time")); audio.onTimeUpdate(() => emit("time"));
      audio.onPlay(() => { if (context === audio) { paused = false; emit("playing"); } });
      audio.onPause(() => { if (context === audio) { paused = true; emit("pause"); } });
      audio.onEnded(() => { if (context === audio) { paused = true; emit("ended"); } });
      audio.onWaiting(() => emit("waiting"));
      audio.onError(() => { if (context === audio) { failed = true; paused = true; emit("error"); } });
      audio.src = url();
    },
    async play() { context?.play(); }, pause() { context?.pause(); },
    removeAttribute() { clear(); }, destroy: clear,
  };
}
