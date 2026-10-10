<script setup lang="ts">
import { ref, computed, onBeforeUnmount, nextTick } from "vue";
import { request, ApiError } from "../lib/api";
type Source = "netease" | "joox" | "audius";
interface Track { id: string; source: Source; name: string; artist: string; genre: string; album: string; version: string; duration: number }
interface Result { tracks: Track[]; has_more: boolean; next_page: number; sources: {source: Source; count: number; error?: string}[] }
const emit = defineEmits<{ (e: "auth-expired"): void }>();
const source = ref<Source | "all">("all");
const sourceOptions = ["all", "netease", "joox", "audius"] as const;
const query = ref("");
const searched = ref("");
const searchedSource = ref<Source | "all">("all");
const tracks = ref<Track[]>([]);
const loading = ref(false);
const error = ref("");
const sourceStatus = ref<Result["sources"]>([]);
const more = ref(false);
const page = ref(1);
const queue = ref<Track[]>([]);
const current = ref(-1);
const selected = computed(() => queue.value[current.value]);
const audio = ref<HTMLAudioElement | null>(null);
const streamURL = ref("");
const resolving = ref(false);
const playback = ref("");
let generation = 0;
let playGeneration = 0;
let disposed = false;
const sourceName = (s: string) => ({netease:"网易云",joox:"JOOX",audius:"独立音乐",all:"华语聚合"}[s] || s);
const trackKey = (t: Track) => t.source + ":" + t.id;
function duration(seconds: number) {
  if (!Number.isFinite(seconds) || seconds <= 0) return "";
  return Math.floor(seconds / 60) + ":" + String(Math.floor(seconds % 60)).padStart(2, "0");
}
async function search(append = false) {
  if (loading.value && append) return;
  const run = ++generation;
  const term = append ? searched.value : query.value.trim();
  if (!term) { error.value = "先输入歌名或歌手，再开饭。"; loading.value = false; return; }
  const usingSource = append ? searchedSource.value : source.value;
  const next = append ? page.value : 1;
  loading.value = true;
  error.value = "";
  if (!append) { tracks.value = []; more.value = false; sourceStatus.value = []; searched.value = term; searchedSource.value = usingSource; }
  try {
    const data = await request<Result>("/canteen/search?q=" + encodeURIComponent(term) + "&source=" + usingSource + "&page=" + next);
    if (disposed || run !== generation) return;
    const previous = append ? tracks.value : [];
    const seen = new Set(previous.map(trackKey));
    tracks.value = [...previous, ...data.tracks.filter(t => { const key = trackKey(t); if (seen.has(key)) return false; seen.add(key); return true; })];
    more.value = data.has_more;
    page.value = data.next_page;
    sourceStatus.value = data.sources;
  } catch (e) {
    if (disposed || run !== generation) return;
    if (e instanceof ApiError && e.status === 401) emit("auth-expired");
    error.value = e instanceof Error ? e.message : "搜歌失败，请重试";
  } finally { if (!disposed && run === generation) loading.value = false; }
}
function chooseSource(s: Source | "all") { source.value = s; if (query.value.trim()) search(); }
function findAlternative() {
  const t = selected.value;
  if (!t) return;
  query.value = t.name;
  source.value = t.source === "netease" ? "joox" : "netease";
  search();
}
async function play(index: number, fromResults = false) {
  // #ifdef H5
  const nextTrack = (fromResults ? tracks.value : queue.value)[index];
  if (!nextTrack) return;
  const run = ++playGeneration;
  resolving.value = true;
  audio.value?.pause();
  audio.value?.removeAttribute("src");
  audio.value?.load();
  streamURL.value = "";
  if (fromResults) queue.value = [...tracks.value];
  current.value = index;
  playback.value = "正在向" + sourceName(nextTrack.source) + "取餐…";
  try {
    const data = await request<{url: string}>("/canteen/playback", "POST", {id: nextTrack.id, source: nextTrack.source});
    if (disposed || run !== playGeneration) return;
    streamURL.value = data.url;
    await nextTick();
    if (disposed || run !== playGeneration || !audio.value) return;
    resolving.value = false;
    playback.value = "音频加载中，马上开饭…";
    audio.value.load();
    await audio.value.play();
  } catch (e) {
    if (disposed || run !== playGeneration) return;
    if (e instanceof ApiError && e.status === 401) emit("auth-expired");
    playback.value = (e as Error).name === "NotAllowedError" ? "点击下方播放按钮开始收听" : e instanceof Error ? e.message : "这首暂时无法播放，请换源试试";
  } finally { if (!disposed && run === playGeneration) resolving.value = false; }
  // #endif
}
function step(delta: number) {
  const next = current.value + delta;
  if (next >= 0 && next < queue.value.length) play(next);
}
function stop() {
  playGeneration++;
  resolving.value = true;
  audio.value?.pause();
  audio.value?.removeAttribute("src");
  audio.value?.load();
  streamURL.value = "";
  current.value = -1;
  queue.value = [];
  playback.value = "";
  resolving.value = false;
}
onBeforeUnmount(() => { disposed = true; generation++; stop(); });
</script>

<template>
  <view class="canteen">
    <view class="hero">
      <text class="eyebrow">MENTAL SNACKS / 耳朵补给站</text>
      <text class="title">精神食粮</text>
      <text class="subtitle">身体在工位，灵魂在打碟。</text>
      <text class="scope">华语多源菜单 · 同一首歌，多一个选择。能否播放以音源实际返回为准。</text>
    </view>
    <!-- #ifdef H5 -->
    <view class="search-box">
      <input v-model="query" class="search-input" maxlength="80" placeholder="搜歌名或歌手，例如十年、周杰伦" confirm-type="search" @confirm="search()" />
      <button class="search-button" @click="search()">{{ loading ? "搜歌中…" : "开饭 · 搜歌" }}</button>
    </view>
    <view class="sources"><button v-for="s in sourceOptions" :key="s" :class="{ chosen: source === s }" @click="chooseSource(s)">{{ sourceName(s) }}</button></view>
    <view v-if="selected" class="now-playing">
      <view class="playing-heading"><text class="disc">♫</text><view class="playing-info"><text class="song-title">{{ selected.name }}</text><text class="artist">{{ selected.artist }} · {{ sourceName(selected.source) }} · {{ selected.version }}</text></view><button class="close" @click="stop">收餐 ×</button></view>
      <text class="play-status" role="status">{{ playback || '给精神充点电。' }}</text>
      <audio ref="audio" :src="streamURL || undefined" controls preload="auto"
        @waiting="playback = '音频缓冲中，耳朵稍等一下…'"
        @playing="playback = '正在播放 · 工作的事等会再说'"
        @pause="!resolving && (playback = '已暂停 · 精神休息中')"
        @error="!resolving && streamURL && (playback = '音频暂时无法加载，可重试或换源找同曲')"
        @ended="step(1)"></audio>
      <view class="player-actions"><button :disabled="current <= 0" @click="step(-1)">上一首</button><button :disabled="resolving" @click="play(current)">重试播放</button><button @click="findAlternative">换源找同曲</button><button :disabled="current >= queue.length - 1" @click="step(1)">下一首</button></view>
    </view>
    <view class="results-heading"><text>今日精神菜单</text><text>{{ tracks.length }} 首{{ searched ? ' · ' + searched : ' · 等待点歌' }}</text></view>
    <view v-if="sourceStatus.length" class="source-status"><text v-for="s in sourceStatus" :key="s.source">{{ sourceName(s.source) }}：{{ s.error || (s.count + ' 条结果') }}</text></view>
    <view v-if="error" class="notice" role="alert">{{ error }}<button @click="search()">再试一次</button></view>
    <view v-else-if="loading && !tracks.length" class="empty" role="status">正在翻找精神补给，请稍等…</view>
    <view v-else-if="!tracks.length" class="empty">{{ searched ? '这个来源暂时没有搜索结果，试试其他关键词或切换来源。' : '输入歌名或歌手，给耳朵加个餐。' }}</view>
    <view class="track-list">
      <button v-for="(track, index) in tracks" :key="trackKey(track)" class="track" :class="{active: selected && trackKey(selected) === trackKey(track)}" @click="play(index, true)">
        <text class="track-number">{{ selected && trackKey(selected) === trackKey(track) ? '♫' : String(index + 1).padStart(2, '0') }}</text>
        <view class="track-info"><text class="song-title">{{ track.name }}</text><text v-if="track.album" class="album">{{ track.album }}</text><text class="artist">{{ track.artist }} · {{ sourceName(track.source) }} · {{ track.version }}</text></view>
        <text class="duration">{{ duration(track.duration) }}</text><text class="play-icon">▶</text>
      </button>
    </view>
    <button v-if="more" class="load-more" :disabled="loading" @click="search(true)">{{ loading ? '正在加菜…' : '继续加菜' }}</button>
    <!-- #endif -->
    <!-- #ifndef H5 -->
    <view class="empty">精神食粮目前支持网页试玩版，请在浏览器打开食堂收听。</view>
    <!-- #endif -->
    <view class="credits"><a href="https://music.gdstudio.xyz/" target="_blank" rel="noopener noreferrer">GD音乐台(music.gdstudio.xyz)</a><text> · 公开接口提供华语搜索与音源；独立音乐来自 Audius。</text></view>
    <text class="footer">精神食粮不限量供应，吃饱了再假装热爱工作。</text>
  </view>
</template>

<style scoped>
.canteen { border: 1px solid #ded7e6; border-radius: 12px; background: #faf9f2; overflow: hidden; }
.hero { padding: 28px 24px 22px; background: #eee6f4; }
.eyebrow { display: block; font-size: 10px; letter-spacing: 2px; color: #917aa7; }
.title { display: block; margin: 12px 0 6px; font-size: 30px; font-weight: 700; color: #654b80; }
.subtitle { display: block; font-size: 14px; color: #7c688f; }
.scope { display: block; margin-top: 16px; font-size: 11px; line-height: 1.8; color: #877891; }
.search-box { display: flex; gap: 10px; padding: 22px 22px 12px; }
.search-input { flex: 1; min-width: 0; height: 42px; padding: 0 12px; background: white; border: 1px solid #ded7e6; border-radius: 7px; font-size: 13px; }
.search-button { margin: 0; border: 0; border-radius: 7px; background: #806096; color: white; font-size: 12px; padding: 0 16px; display: flex; align-items: center; }
.sources { display: flex; flex-wrap: wrap; gap: 8px; padding: 0 22px 12px; }
.sources button { margin: 0; padding: 7px 12px; font-size: 12px; border: 1px solid #d8cbe2; border-radius: 6px; color: #877891; background: #faf9f2; }
.sources button.chosen { color: #fff; background: #806096; }
.source-status { display: flex; flex-wrap: wrap; gap: 12px; padding: 12px 22px; color: #97947e; font-size: 11px; }
.album { display: block; font-size: 10px; color: #a6a28f; margin-top: 3px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.credits { padding: 18px 20px 0; font-size: 10px; color: #a19aab; line-height: 1.8; }
.credits a { color: #95819f; }
.player-actions button, .load-more, .notice button { margin: 0; padding: 7px 12px; border: 1px solid #dcd8c9; border-radius: 6px; font-size: 11px; color: #777d60; background: #f2f1e6; }
.now-playing { margin: 0 22px 20px; padding: 16px; border: 1px solid #d8cbe2; border-radius: 9px; background: #f3edf7; }
.playing-heading { display: flex; align-items: center; gap: 14px; }
.disc { font-size: 30px; color: #9471b0; }
.playing-info { flex: 1; min-width: 0; }
.close { margin: 0; background: transparent; border: 0; color: #97839f; font-size: 11px; padding: 4px; }
.play-status { display: block; margin: 12px 0; font-size: 11px; color: #8c769b; }
audio { display: block; width: 100%; height: 42px; }
.player-actions { display: flex; flex-wrap: wrap; justify-content: center; gap: 12px; margin-top: 12px; }
button:disabled { opacity: .45; }
.results-heading { display: flex; justify-content: space-between; gap: 10px; padding: 12px 22px; font-size: 12px; color: #8c9177; border-bottom: 1px solid #e7e5d9; }
.track { display: flex; align-items: center; gap: 14px; width: 100%; text-align: left; margin: 0; padding: 15px 22px; background: transparent; border: 0; border-bottom: 1px solid #eeece1; border-radius: 0; line-height: 1.5; }
.track:hover, .track.active { background: #eee8f4; }
.track-number { font-size: 12px; color: #a28cb4; width: 22px; flex-shrink: 0; }
.track-info { flex: 1; min-width: 0; }
.song-title { display: block; font-size: 13px; color: #62526f; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.artist { display: block; font-size: 11px; color: #959981; margin-top: 4px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.duration { color: #9b9f87; font-size: 11px; }
.play-icon { color: #9a80af; font-size: 11px; }
.empty, .notice { padding: 30px 22px; font-size: 13px; line-height: 1.8; color: #92967c; text-align: center; }
.notice button { margin: 12px auto 0; width: 100px; }
.load-more { margin: 18px auto; width: 150px; }
.footer { display: block; padding: 20px; text-align: center; font-size: 11px; color: #9b9f88; }
@media (max-width: 760px) { .hero { padding: 22px 16px; } .search-box { padding: 16px 12px 12px; gap: 6px; } .search-button { padding: 0 10px; } .track { padding: 14px 12px; gap: 9px; } .now-playing { margin: 0 12px 16px; padding: 12px; } .duration { display: none; } }
</style>
