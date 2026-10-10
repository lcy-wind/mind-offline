<script setup lang="ts">
import { ref } from "vue";
import MusicPanel from "./MusicPanel.vue";
import MusicCanteen from "./MusicCanteen.vue";
import { player, playerState, type MusicProvider } from "../lib/player";
const props = defineProps<{ accountId: string; foreground: boolean }>();
const emit = defineEmits<{ (e: "auth-expired"): void }>();
const active = ref<MusicProvider | "canteen">(
  playerState.track ? playerState.provider : "netease",
);
function select(provider: MusicProvider | "canteen") {
  if (active.value !== provider) {
    player.reset();
    active.value = provider;
  }
}
</script>
<template>
  <view class="music-platforms"
    ><button
      :class="{ selected: active === 'netease' }"
      @click="select('netease')"
    >
      <text>♫</text> 网易云音乐</button
    ><button :class="{ selected: active === 'kugou' }" @click="select('kugou')">
      <text>♪</text> 酷狗音乐 <text class="platform-beta">实验</text></button
    ><button
      :class="{ selected: active === 'canteen' }"
      @click="select('canteen')"
    >
      <text>⌕</text> 音乐食堂
    </button></view
  ><view class="platform-hint"
    >网易云、酷狗账号分别绑定，切换栏目会停止当前播放。</view
  ><MusicCanteen
    v-if="active === 'canteen'"
    :key="props.accountId"
    @auth-expired="emit('auth-expired')"
  /><MusicPanel
    v-else
    :key="props.accountId + active"
    :account-id="props.accountId"
    :foreground="props.foreground"
    :provider="active"
    @auth-expired="emit('auth-expired')"
  />
</template>
<style scoped>
.music-platforms {
  display: flex;
  gap: 10px;
  margin: 8px 0 12px;
}
.music-platforms button {
  display: flex;
  align-items: center;
  gap: 8px;
  border: 1px solid #d9d9cb;
  border-radius: 7px;
  padding: 12px 20px;
  font-size: 12px;
  color: #929681;
  background: #faf9f2;
  cursor: pointer;
}
.music-platforms button.selected {
  background: #e9e1f1;
  border-color: #b6a3ce;
  color: #745693;
  font-weight: 600;
}
.music-platforms button > text:first-child {
  font-size: 19px;
}
.platform-beta {
  font-size: 8px;
  padding: 2px 5px;
  border-radius: 3px;
  background: #e3edc6;
  color: #8a9a65;
}
.platform-hint {
  font-size: 10px;
  color: #9ca48c;
  margin-bottom: 20px;
}
@media (max-width: 760px) {
  .music-platforms button {
    padding: 10px 7px;
    font-size: 10px;
    flex: 1;
    justify-content: center;
    gap: 4px;
  }
  .platform-hint {
    font-size: 9px;
  }
}
</style>
