<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted, nextTick } from "vue";
import { request, ApiError } from "../lib/api";
import {
  player,
  playerState,
  formatTime,
  type MusicTrack,
  type MusicProvider,
} from "../lib/player";
const selectedPlaylist = ref<Playlist | null>(null),
  tracks = ref<MusicTrack[]>([]),
  tracksLoading = ref(false),
  tracksMore = ref(false),
  tracksTotal = ref(0);
let trackOffset = 0,
  trackRevision = 0;
async function openTracks(p: Playlist) {
  selectedPlaylist.value = p;
  tracks.value = [];
  trackOffset = 0;
  tracksMore.value = false;
  tracksTotal.value = 0;
  await loadTracks(false);
  if (alive && selectedPlaylist.value?.id === p.id) {
    await nextTick();
    uni.pageScrollTo({ selector: "#music-tracks", duration: 250 });
  }
}
async function loadTracks(append = false) {
  const p = selectedPlaylist.value;
  if (!p) return;
  const rev = ++trackRevision;
  tracksLoading.value = true;
  try {
    const result = await request<{
      items: MusicTrack[];
      total: number;
      offset: number;
      more: boolean;
    }>(
      base +
        "/tracks?playlist_id=" +
        p.id +
        "&offset=" +
        (append ? trackOffset : 0),
    );
    if (!alive || rev !== trackRevision) return;
    tracks.value = append ? [...tracks.value, ...result.items] : result.items;
    tracksTotal.value = result.total;
    tracksMore.value = result.more;
    trackOffset = result.offset + 30;
    player.extendQueue(p.id, tracks.value, provider);
    error.value = "";
  } catch (e) {
    handleError(e);
  } finally {
    if (alive && rev === trackRevision) tracksLoading.value = false;
  }
}
function playTrack(t: MusicTrack) {
  if (selectedPlaylist.value)
    void player.select(t, tracks.value, selectedPlaylist.value.id, provider);
}
const props = defineProps<{
  accountId: string;
  foreground: boolean;
  provider?: MusicProvider;
}>();
const provider = props.provider || "netease";
const platformName = provider === "kugou" ? "酷狗音乐" : "网易云音乐";
const base = "/music/" + provider;
function stopProvider() {
  if (playerState.provider === provider) player.reset();
}
const emit = defineEmits<{ (e: "auth-expired"): void }>();
type Binding = {
  enabled: boolean;
  bound: boolean;
  uid?: string;
  nickname?: string;
  avatar?: string;
  bound_at?: string;
  expired?: boolean;
};
type Playlist = {
  id: string;
  name: string;
  cover: string;
  track_count: number;
  created: boolean;
  url: string;
};
const binding = ref<Binding | null>(null),
  loading = ref(true),
  busy = ref(false),
  error = ref(""),
  consent = ref(false),
  qr = ref<{ attempt_id: string; image: string; expires_at: string } | null>(
    null,
  ),
  qrState = ref("waiting"),
  polling = ref(false),
  paused = ref(false),
  now = ref(Date.now());
const playlists = ref<Playlist[]>([]),
  playlistLoading = ref(false),
  more = ref(false),
  offset = ref(0);
const remaining = computed(() =>
  qr.value
    ? Math.max(
        0,
        Math.ceil((new Date(qr.value.expires_at).getTime() - now.value) / 1000),
      )
    : 0,
);
const captions: Record<string, string> = {
  waiting: "打开" + platformName + " App，扫一扫",
  confirming: "扫码成功，请在" + platformName + " App 中确认登录",
  expired: "二维码已过期，请重新获取",
  bound: "绑定成功，正在读取你的歌单",
};
let alive = true,
  timer: ReturnType<typeof setInterval> | undefined,
  nextPoll = 0,
  revision = 0;
function handleError(e: unknown) {
  if (!alive) return;
  if (e instanceof ApiError && e.status === 401) {
    emit("auth-expired");
    return;
  }
  if (!(e instanceof ApiError && e.status === 0))
    error.value = (e as Error).message;
}
async function retryStatus() {
  error.value = "";
  loading.value = true;
  await loadStatus();
  if (alive && binding.value?.bound && !binding.value.expired)
    await loadPlaylists(0);
}
async function loadStatus() {
  try {
    const data = await request<Binding>(base);
    if (alive) {
      binding.value = data;
      if (!data.bound || data.expired) stopProvider();
    }
  } catch (e) {
    handleError(e);
  } finally {
    if (alive) loading.value = false;
  }
}
async function loadPlaylists(newOffset = 0) {
  if (playlistLoading.value) return;
  playlistLoading.value = true;
  const rev = revision;
  try {
    const data = await request<{
      items: Playlist[];
      more: boolean;
      offset: number;
    }>(base + "/playlists?offset=" + newOffset);
    if (!alive || rev !== revision) return;
    playlists.value = data.items;
    more.value = data.more;
    offset.value = data.offset;
    error.value = "";
  } catch (e) {
    handleError(e);
    if (e instanceof ApiError && e.status === 409) {
      playlists.value = [];
      await loadStatus();
    }
  } finally {
    if (alive) playlistLoading.value = false;
  }
}
async function begin() {
  if (!consent.value || busy.value) return;
  busy.value = true;
  error.value = "";
  const rev = ++revision;
  qr.value = null;
  paused.value = false;
  try {
    const data = await request<{
      attempt_id: string;
      image: string;
      expires_at: string;
    }>(base + "/qr", "POST");
    if (!alive || rev !== revision) {
      request(base + "/qr/cancel", "POST", {
        attempt_id: data.attempt_id,
      }).catch(() => {});
      return;
    }
    qr.value = data;
    qrState.value = "waiting";
    now.value = Date.now();
    nextPoll = Date.now() + 3000;
  } catch (e) {
    handleError(e);
  } finally {
    if (alive) busy.value = false;
  }
}
async function poll() {
  if (
    !qr.value ||
    polling.value ||
    paused.value ||
    !props.foreground ||
    ["expired", "bound"].includes(qrState.value)
  )
    return;
  const id = qr.value.attempt_id,
    rev = revision;
  polling.value = true;
  try {
    const result = await request<{ status: string }>(
      base + "/qr/check",
      "POST",
      { attempt_id: id },
    );
    if (!alive || rev !== revision) return;
    qrState.value = result.status;
    error.value = "";
    if (result.status === "bound") {
      stopProvider();
      qr.value = null;
      await loadStatus();
      await loadPlaylists();
    }
  } catch (e) {
    if (!alive || rev !== revision) return;
    if (e instanceof ApiError && e.status === 410) qrState.value = "expired";
    else {
      paused.value = true;
      handleError(e);
    }
  } finally {
    if (alive) polling.value = false;
    nextPoll = Date.now() + 3000;
  }
}
async function cancel() {
  if (!qr.value) return;
  const id = qr.value.attempt_id;
  ++revision;
  qr.value = null;
  error.value = "";
  try {
    await request(base + "/qr/cancel", "POST", { attempt_id: id });
  } catch (e) {
    handleError(e);
  }
}
async function unbind() {
  if (busy.value) return;
  const confirmation = await new Promise<boolean>((resolve) =>
    uni.showModal({
      title: "解除" + platformName + "绑定？",
      content:
        "将删除本站保存的" + platformName + "登录凭证，不会修改你的歌单。",
      confirmText: "解除绑定",
      success: (r) => resolve(Boolean(r.confirm)),
      fail: () => resolve(false),
    }),
  );
  if (!confirmation || !alive) return;
  busy.value = true;
  ++revision;
  try {
    await request(base + "/unbind", "POST");
    if (!alive) return;
    qr.value = null;
    stopProvider();
    selectedPlaylist.value = null;
    tracks.value = [];
    ++trackRevision;
    binding.value = { enabled: true, bound: false };
    playlists.value = [];
    offset.value = 0;
    more.value = false;
    consent.value = false;
    error.value = "";
  } catch (e) {
    handleError(e);
  } finally {
    if (alive) busy.value = false;
  }
}
function openPlaylist(p: Playlist) {
  // #ifdef H5
  window.open(p.url, "_blank", "noopener,noreferrer");
  // #endif
  // #ifndef H5
  uni.setClipboardData({
    data: p.url,
    success: () =>
      uni.showToast({ title: "链接已复制，请在浏览器打开", icon: "none" }),
  });
  // #endif
}
onMounted(async () => {
  await loadStatus();
  if (alive && binding.value?.bound && !binding.value.expired)
    await loadPlaylists();
  if (!alive) return;
  timer = setInterval(() => {
    if (!props.foreground) return;
    now.value = Date.now();
    if (qr.value && remaining.value === 0) {
      qrState.value = "expired";
      return;
    }
    if (Date.now() >= nextPoll) poll();
  }, 1000);
});
onUnmounted(() => {
  alive = false;
  clearInterval(timer);
  const id = qr.value?.attempt_id;
  if (id)
    request(base + "/qr/cancel", "POST", { attempt_id: id }).catch(() => {});
  ++revision;
});
</script>
<template>
  <view class="music-lab"
    ><view class="music-hero"
      ><view
        ><text class="eyebrow">MIND OFFLINE / MUSIC LAB</text
        ><view class="music-title">耳机一戴，<br /><text>工位之外。</text></view
        ><text class="music-subtitle"
          >把你的{{ platformName }}歌单，带进精神离职食堂。</text
        ></view
      ><view class="record"><view class="record-center">♫</view></view
      ><text class="music-beta">实验接入 · BETA</text></view
    >
    <view v-if="error" class="error-banner"
      >{{ error
      }}<button
        v-if="!qr"
        class="outline"
        :disabled="loading || busy"
        @click="retryStatus"
      >
        重试连接
      </button></view
    >
    <view v-if="loading" class="music-card music-empty"
      >正在打开音乐实验室…</view
    >
    <view v-else-if="binding && !binding.enabled" class="music-card music-empty"
      >音乐实验室暂未启用，请稍后再来。</view
    >
    <template v-else-if="binding?.enabled">
      <view v-if="binding.bound" class="music-card"
        ><view class="music-profile"
          ><image
            v-if="binding.avatar"
            :src="binding.avatar"
            class="music-avatar"
            mode="aspectFill"
          /><view v-else class="music-avatar avatar-placeholder">♫</view
          ><view class="music-person"
            ><text class="music-nickname">{{ binding.nickname }}</text
            ><text class="muted">{{ platformName }} ID：{{ binding.uid }}</text
            ><text class="music-binding-label">{{
              binding.expired
                ? "登录已失效，需要重新扫码"
                : "✓ 已绑定到当前食堂账号"
            }}</text></view
          ><button class="outline" :disabled="busy" @click="unbind">
            解除绑定
          </button></view
        ><text v-if="binding.expired" class="music-expired"
          >{{ platformName }}登录已过期或失效，你的食堂账号不受影响。</text
        ></view
      >
      <view
        v-if="!binding.bound || binding.expired || qr"
        class="music-card music-connect"
        ><view
          ><text class="eyebrow">CONNECT YOUR MUSIC ACCOUNT</text
          ><view class="section-title">{{
            binding.expired ? "让音乐重新上线。" : "扫码，带上你的歌单。"
          }}</view
          ><text class="muted"
            >建议在电脑上打开本页，用手机{{ platformName }} App 扫码。</text
          ></view
        >
        <view v-if="!qr" class="music-consent"
          ><checkbox-group
            @change="consent = $event.detail.value.includes('agree')"
            ><label
              ><checkbox
                value="agree"
                :checked="consent"
                color="#6550a0"
              /><text
                >我同意通过第三方实验接口登录{{
                  platformName
                }}，并由本站加密保存登录凭证，用于读取我的账号资料和歌单。</text
              ></label
            ></checkbox-group
          ><text class="music-disclosure"
            >这是扫码登录，不是{{ platformName }}官方 OAuth
            授权。无需在本站输入{{ platformName }}密码；绑定可以随时解除。</text
          ><button class="primary" :disabled="!consent || busy" @click="begin">
            {{
              busy ? "正在生成二维码…" : "生成" + platformName + "登录二维码 →"
            }}
          </button></view
        >
        <view v-else class="qr-layout"
          ><view class="qr-frame" :class="{ expired: qrState === 'expired' }"
            ><image :src="qr.image" mode="aspectFit" class="qr-image" /></view
          ><view class="qr-instructions"
            ><text class="qr-state">{{
              captions[qrState] || "正在查询登录状态"
            }}</text
            ><text class="muted">{{
              qrState === "expired"
                ? "过期二维码无法继续绑定。"
                : "二维码剩余约 " + remaining + " 秒，仅限当前账号和登录会话。"
            }}</text
            ><view class="qr-buttons"
              ><button
                v-if="qrState === 'expired'"
                class="primary"
                :disabled="busy"
                @click="begin"
              >
                重新获取</button
              ><button
                v-else-if="paused"
                class="primary"
                @click="
                  paused = false;
                  error = '';
                  poll();
                "
              >
                重试查询</button
              ><button class="outline" @click="cancel">取消绑定</button></view
            ><text class="music-disclosure"
              >如果{{
                platformName
              }}提示设备环境异常，请停止本次尝试，稍后再试。本实验不会绕过平台验证。</text
            ></view
          ></view
        ></view
      >
      <view v-if="binding.bound && !binding.expired" class="music-card"
        ><view class="music-section-heading"
          ><view
            ><text class="eyebrow">YOUR PERSONAL PLAYLISTS</text
            ><view class="section-title">我的摸鱼歌单</view></view
          ><button
            class="outline"
            :disabled="playlistLoading || busy"
            @click="loadPlaylists(0)"
          >
            {{ playlistLoading ? "读取中…" : "↻ 刷新歌单" }}
          </button></view
        ><text class="music-disclosure"
          >点开歌单即可在食堂听歌；有试听限制会标注，无法播放时可前往{{
            platformName
          }}。</text
        ><view v-if="!playlists.length" class="music-empty">{{
          playlistLoading
            ? "正在读取你的歌单…"
            : "暂无可展示的歌单，或暂时读取失败。"
        }}</view
        ><view class="playlist-grid"
          ><view v-for="p in playlists" :key="p.id" class="playlist-card"
            ><image
              v-if="p.cover"
              :src="p.cover"
              mode="aspectFill"
              class="playlist-cover"
            /><view v-else class="playlist-cover cover-placeholder">♫</view
            ><view class="playlist-copy"
              ><text class="playlist-name">{{ p.name }}</text
              ><text class="muted"
                >{{ p.created ? "我创建的" : "我收藏的" }} ·
                {{ p.track_count }} 首</text
              ><button class="playlist-listen" @click="openTracks(p)">
                打开歌曲列表 →</button
              ><button class="playlist-link" @click="openPlaylist(p)">
                前往{{ platformName }} ↗
              </button></view
            ></view
          ></view
        ><view v-if="offset || more" class="music-pagination"
          ><text class="muted">第 {{ Math.floor(offset / 20) + 1 }} 页</text
          ><button
            class="outline"
            :disabled="!offset || playlistLoading"
            @click="loadPlaylists(Math.max(0, offset - 20))"
          >
            上一页</button
          ><button
            class="outline"
            :disabled="!more || playlistLoading"
            @click="loadPlaylists(offset + 20)"
          >
            下一页
          </button></view
        ></view
      >
      <view
        v-if="selectedPlaylist && binding.bound && !binding.expired"
        id="music-tracks"
        class="music-card track-section"
        ><view class="music-section-heading"
          ><view
            ><text class="eyebrow">PLAYLIST / {{ tracksTotal }} 首</text
            ><view class="section-title">{{
              selectedPlaylist.name
            }}</view></view
          ><button
            class="outline"
            :disabled="tracksLoading"
            @click="loadTracks(false)"
          >
            刷新
          </button></view
        ><text class="music-disclosure"
          >点击歌曲开始播放，按顺序播放已加载的歌曲；切回菜单可继续听。</text
        ><view v-if="!tracks.length" class="music-empty">{{
          tracksLoading
            ? "正在读取歌曲…"
            : "暂无可读取的歌曲，可重试或前往" + platformName + "。"
        }}</view
        ><view
          v-for="(t, i) in tracks"
          :key="t.id"
          class="track-row"
          :class="{
            current:
              playerState.provider === provider &&
              playerState.track?.id === t.id,
          }"
          ><text class="track-number">{{
            playerState.provider === provider &&
            playerState.track?.id === t.id &&
            playerState.status === "playing"
              ? "♫"
              : String(i + 1).padStart(2, "0")
          }}</text
          ><image
            v-if="t.cover"
            :src="t.cover"
            mode="aspectFill"
            class="track-cover"
          /><view class="track-copy"
            ><text class="track-name">{{ t.name }}</text
            ><text class="muted">{{ t.artist }} · {{ t.album }}</text
            ><text
              v-if="
                playerState.provider === provider &&
                playerState.track?.id === t.id
              "
              class="track-feedback"
              >{{ playerState.trial ? "试听 · " : ""
              }}{{ playerState.message }}</text
            ></view
          ><text class="track-duration">{{ formatTime(t.duration) }}</text
          ><button
            class="track-play"
            :aria-label="'播放' + t.name"
            @click="
              playerState.provider === provider &&
              playerState.track?.id === t.id
                ? player.toggle()
                : playTrack(t)
            "
          >
            {{
              playerState.provider === provider &&
              playerState.track?.id === t.id &&
              playerState.status === "playing"
                ? "Ⅱ"
                : "▶"
            }}
          </button></view
        ><button
          v-if="tracksMore"
          class="outline load-tracks"
          :disabled="tracksLoading"
          @click="loadTracks(true)"
        >
          {{ tracksLoading ? "加载中…" : "加载更多歌曲" }}
        </button></view
      >
    </template>
    <view class="music-footer"
      >第三方接口可能变化或受风控影响。每位顾客的绑定独立保存，店长后台不展示音乐登录凭证。</view
    >
  </view>
</template>
<style scoped>
.playlist-listen {
  font-size: 11px;
  text-align: left;
  color: #6f5199;
  font-weight: 600;
  padding: 5px 0;
}
.track-section .section-title {
  font-size: 20px;
  word-break: break-all;
}
.track-row {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 14px 5px;
  border-bottom: 1px solid #e4e7d7;
}
.track-row.current {
  background: #eee8f5;
  border-radius: 6px;
}
.track-number {
  font-size: 10px;
  color: #a0aa8e;
  width: 21px;
  flex-shrink: 0;
  text-align: center;
}
.track-cover {
  height: 36px;
  width: 36px;
  border-radius: 4px;
  flex-shrink: 0;
}
.track-copy {
  flex: 1;
  min-width: 0;
}
.track-name {
  font-size: 12px;
  font-weight: 600;
  display: block;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.track-copy .muted {
  font-size: 9px;
  display: block;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  margin-top: 5px;
}
.track-feedback {
  font-size: 9px;
  color: #9671ae;
  display: block;
  margin-top: 4px;
  line-height: 1.6;
}
.track-duration {
  font-size: 9px;
  color: #a0aa8f;
}
.track-play {
  width: 30px;
  height: 30px;
  background: #e1ecc4;
  color: #6b8050;
  border-radius: 50%;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 12px;
  flex-shrink: 0;
}
.load-tracks {
  display: block;
  width: 100%;
  margin-top: 18px;
}
@media (max-width: 760px) {
  .track-row {
    gap: 8px;
  }
  .track-duration {
    display: none;
  }
  .track-cover {
    width: 30px;
    height: 30px;
  }
  .track-number {
    width: 17px;
  }
  .track-name {
    font-size: 11px;
  }
}

.music-hero {
  background: #e8dfef;
  border: 1px solid #d8cbe4;
  border-radius: 10px;
  padding: 32px;
  position: relative;
  display: flex;
  justify-content: space-between;
  overflow: hidden;
  margin-bottom: 20px;
  min-height: 230px;
}
.music-title {
  font-size: 35px;
  font-weight: 850;
  line-height: 1.4;
  margin: 18px 0 13px;
}
.music-title text {
  color: #7658a2;
}
.music-subtitle {
  font-size: 12px;
  color: #9583a3;
  line-height: 1.8;
}
.record {
  width: 170px;
  height: 170px;
  border-radius: 50%;
  background: repeating-radial-gradient(
    circle at center,
    #35392f 0,
    #35392f 4px,
    #43483b 5px,
    #35392f 6px
  );
  display: flex;
  align-items: center;
  justify-content: center;
  box-shadow: 10px 10px 0 #cbbfdb;
  transform: rotate(15deg);
  align-self: center;
  margin: 10px 18px 0 20px;
  flex-shrink: 0;
}
.record-center {
  width: 61px;
  height: 61px;
  border-radius: 50%;
  background: #d6e794;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 31px;
  color: #4f6134;
}
.music-beta {
  position: absolute;
  right: 18px;
  top: 18px;
  font-size: 9px;
  color: #9c8bae;
  letter-spacing: 1px;
}
.music-card {
  background: #faf9f2;
  border: 1px solid #d9dfcb;
  border-radius: 9px;
  padding: 26px;
  margin: 18px 0;
}
.music-empty {
  text-align: center;
  font-size: 12px;
  color: #97a183;
  padding: 38px 15px;
  line-height: 1.8;
}
.music-consent {
  margin-top: 24px;
}
.music-consent label {
  display: flex;
  align-items: flex-start;
  gap: 10px;
  font-size: 12px;
  line-height: 1.9;
  color: #7f806e;
}
.music-consent checkbox {
  transform: scale(0.8);
  transform-origin: top left;
  flex-shrink: 0;
  width: 22px;
  margin-top: 1px;
}
.music-disclosure {
  font-size: 10px;
  line-height: 1.9;
  color: #a0a58f;
  display: block;
  margin: 12px 0;
}
.music-consent .primary {
  margin-top: 20px;
  width: 100%;
  max-width: 360px;
}
.qr-layout {
  display: flex;
  align-items: center;
  gap: 25px;
  margin-top: 25px;
}
.qr-frame {
  background: white;
  border: 1px solid #ddd4e6;
  border-radius: 8px;
  padding: 10px;
  flex-shrink: 0;
}
.qr-frame.expired {
  opacity: 0.35;
}
.qr-image {
  width: 205px;
  height: 205px;
  display: block;
}
.qr-instructions {
  min-width: 0;
  flex: 1;
}
.qr-state {
  font-size: 16px;
  color: #705793;
  line-height: 1.7;
  display: block;
  font-weight: 600;
  margin-bottom: 10px;
}
.qr-buttons {
  display: flex;
  gap: 10px;
  align-items: center;
  margin: 20px 0;
}
.qr-buttons .primary {
  font-size: 11px;
  padding: 11px 17px;
}
.music-profile {
  display: flex;
  align-items: center;
  gap: 16px;
}
.music-avatar {
  width: 58px;
  height: 58px;
  border-radius: 50%;
  flex-shrink: 0;
}
.avatar-placeholder {
  background: #e9e0f1;
  color: #9979b7;
  font-size: 28px;
  display: flex;
  align-items: center;
  justify-content: center;
}
.music-person {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 5px;
}
.music-nickname {
  font-size: 19px;
  font-weight: 750;
  word-break: break-all;
}
.music-binding-label {
  font-size: 10px;
  color: #8c9f63;
}
.music-profile .outline {
  font-size: 10px;
  flex-shrink: 0;
}
.music-expired {
  background: #f3e8d9;
  color: #a18555;
  padding: 12px;
  border-radius: 5px;
  font-size: 11px;
  display: block;
  margin-top: 18px;
  line-height: 1.8;
}
.music-section-heading {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}
.music-section-heading .section-title {
  font-size: 21px;
}
.music-section-heading .outline {
  font-size: 10px;
  white-space: nowrap;
}
.playlist-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 14px;
  margin-top: 22px;
}
.playlist-card {
  display: flex;
  gap: 12px;
  padding: 12px;
  border: 1px solid #e0e4d4;
  border-radius: 6px;
  min-width: 0;
}
.playlist-cover {
  height: 72px;
  width: 72px;
  border-radius: 5px;
  flex-shrink: 0;
}
.cover-placeholder {
  background: #e2e9d1;
  color: #91a075;
  font-size: 30px;
  display: flex;
  align-items: center;
  justify-content: center;
}
.playlist-copy {
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 5px;
  flex: 1;
}
.playlist-name {
  font-size: 12px;
  font-weight: 600;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.playlist-copy .muted {
  font-size: 9px;
}
.playlist-link {
  font-size: 10px;
  color: #7b5ca2;
  align-self: flex-start;
  margin-top: auto;
  text-align: left;
}
.music-pagination {
  display: flex;
  gap: 10px;
  justify-content: flex-end;
  align-items: center;
  margin-top: 22px;
}
.music-pagination > text {
  margin-right: auto;
}
.music-footer {
  font-size: 9px;
  color: #a1a68e;
  line-height: 1.9;
  padding: 8px 3px 25px;
}
@media (max-width: 1050px) {
  .music-hero {
    padding: 25px;
  }
  .record {
    width: 135px;
    height: 135px;
    margin-right: 0;
  }
  .music-title {
    font-size: 31px;
  }
  .qr-layout {
    gap: 18px;
  }
  .qr-image {
    width: 170px;
    height: 170px;
  }
  .playlist-grid {
    grid-template-columns: 1fr;
  }
  .music-card {
    padding: 22px;
  }
}
@media (max-width: 760px) {
  .music-hero {
    padding: 25px 20px;
    min-height: 220px;
  }
  .music-title {
    font-size: 30px;
  }
  .music-subtitle {
    font-size: 10px;
    max-width: 180px;
    display: block;
  }
  .record {
    position: absolute;
    right: -16px;
    top: 72px;
    width: 133px;
    height: 133px;
    opacity: 0.85;
  }
  .record-center {
    width: 48px;
    height: 48px;
    font-size: 25px;
  }
  .music-beta {
    font-size: 8px;
    right: 12px;
    top: 15px;
  }
  .music-card {
    padding: 20px 17px;
  }
  .music-consent label {
    font-size: 11px;
  }
  .qr-layout {
    flex-direction: column;
    align-items: stretch;
    gap: 20px;
  }
  .qr-frame {
    align-self: center;
  }
  .qr-image {
    width: 218px;
    height: 218px;
  }
  .qr-state {
    font-size: 14px;
  }
  .qr-instructions {
    text-align: center;
  }
  .qr-buttons {
    justify-content: center;
  }
  .music-avatar {
    width: 44px;
    height: 44px;
  }
  .music-profile {
    gap: 10px;
  }
  .music-nickname {
    font-size: 16px;
  }
  .music-profile .outline {
    font-size: 9px;
    padding: 7px;
  }
  .music-binding-label {
    font-size: 9px;
  }
  .music-person .muted {
    font-size: 9px;
  }
  .playlist-cover {
    height: 65px;
    width: 65px;
  }
  .playlist-grid {
    gap: 11px;
  }
  .music-section-heading .section-title {
    font-size: 20px;
  }
  .music-section-heading .outline {
    font-size: 9px;
  }
  .music-consent .primary {
    max-width: none;
  }
}
</style>
