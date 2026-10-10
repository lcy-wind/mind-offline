<script setup lang="ts">
import { ref, watch, nextTick, onMounted, onBeforeUnmount, computed, getCurrentInstance } from "vue";
const props = defineProps<{ text: string; secondary: string; lineKey: string; playing: boolean }>();
const clip = ref<HTMLDivElement | null>(null);
const line = ref<HTMLSpanElement | null>(null);
const overflow = ref(0);
let observer: ResizeObserver | undefined;
let alive = true;
const instance = getCurrentInstance();
function measure() {
  if (!alive) return;
  // #ifdef H5
  overflow.value = Math.max(0, (line.value?.scrollWidth || 0) - (clip.value?.clientWidth || 0));
  // #endif
  // #ifdef MP-WEIXIN
  const key = props.lineKey;
  uni.createSelectorQuery().in(instance?.proxy).select(".lyric-clip").boundingClientRect().select(".lyric-text").boundingClientRect().exec((rects: any[]) => {
    if (alive && key === props.lineKey) overflow.value = Math.max(0, (rects[1]?.width || 0) - (rects[0]?.width || 0));
  });
  // #endif
}
const motion = computed(() => ({
  "--lyric-shift": -overflow.value + "px",
  "--lyric-duration": Math.max(5, overflow.value / 25 + 3) + "s",
  animationPlayState: props.playing ? "running" : "paused",
}));
watch(() => [props.text, props.lineKey], async () => {
  overflow.value = 0;
  await nextTick();
  measure();
});
onMounted(() => {
  measure();
  // #ifdef H5
  if (typeof ResizeObserver !== "undefined" && clip.value) {
    observer = new ResizeObserver(measure);
    observer.observe(clip.value);
  }
  // #endif
});
onBeforeUnmount(() => { alive = false; observer?.disconnect(); });
</script>

<template>
  <div class="top-lyrics-panel" aria-label="当前歌词">
    <div ref="clip" class="lyric-clip" :class="{ overflowing: overflow > 1 }">
      <div :key="lineKey" class="lyric-enter">
        <span ref="line" class="lyric-text" :class="{ scrolling: overflow > 1 }" :style="motion" :title="text">{{ text }}</span>
      </div>
    </div>
    <div class="lyric-secondary" :title="secondary">{{ secondary || '♫ 精神食粮，持续供应' }}</div>
  </div>
</template>

<style scoped>
.top-lyrics-panel { min-width: 0; text-align: center; padding: 3px 12px; box-sizing: border-box; }
.lyric-clip { overflow: hidden; width: 100%; line-height: 23px; }
.lyric-clip.overflowing { text-align: left; }
.lyric-enter { animation: lyric-rise .3s ease-out both; }
.lyric-text { display: inline-block; width: max-content; max-width: none; white-space: nowrap; color: #76528f; font-size: 14px; font-weight: 600; }
.lyric-text.scrolling { animation: lyric-pan var(--lyric-duration) ease-in-out infinite alternate; }
.lyric-secondary { margin-top: 3px; color: #ad9bb7; font-size: 10px; line-height: 16px; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
@keyframes lyric-rise { from { opacity: 0; transform: translateY(9px); } to { opacity: 1; transform: translateY(0); } }
@keyframes lyric-pan { 0%, 18% { transform: translateX(0); } 82%, 100% { transform: translateX(var(--lyric-shift)); } }
@media (prefers-reduced-motion: reduce) { .lyric-enter, .lyric-text.scrolling { animation: none; } }
@media (max-width: 760px) { .top-lyrics-panel { padding: 5px 6px 0; } .lyric-text { font-size: 12px; } }
</style>
