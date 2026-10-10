<script setup lang="ts">
import { player, playerState as p, formatTime } from "../lib/player";
const emit = defineEmits<{ (e: "open"): void }>();
function external() {
  if (!p.track) return;
  const url = "https://music.163.com/#/song?id=" + p.track.id;
  // #ifdef H5
  window.open(url, "_blank", "noopener,noreferrer");
  // #endif
  // #ifndef H5
  uni.setClipboardData({ data: url });
  // #endif
}
</script>
<template>
  <view v-if="p.track" class="mini-player"
    ><view class="player-top"
      ><image
        v-if="p.track.cover"
        :src="p.track.cover"
        mode="aspectFill"
        class="player-cover"
      /><view v-else class="player-cover player-placeholder">♫</view
      ><view class="player-info" @click="emit('open')"
        ><view class="player-title"
          >{{ p.track.name
          }}<text v-if="p.trial" class="trial-tag">试听</text></view
        ><text class="player-artist">{{ p.track.artist }}</text></view
      ><view class="player-controls"
        ><button
          :disabled="p.index <= 0"
          aria-label="上一首"
          @click="player.move(-1)"
        >
          ⏮</button
        ><button
          class="player-toggle"
          :aria-label="p.status === 'playing' ? '暂停' : '播放'"
          @click="player.toggle"
        >
          {{
            p.status === "loading" ? "…" : p.status === "playing" ? "Ⅱ" : "▶"
          }}</button
        ><button
          :disabled="p.index >= p.queue.length - 1"
          aria-label="下一首"
          @click="player.move(1)"
        >
          ⏭
        </button></view
      ><button
        class="player-close"
        aria-label="关闭播放器"
        @click="player.reset"
      >
        ×
      </button></view
    ><view class="player-progress"
      ><text>{{ formatTime(p.current) }}</text
      ><slider
        :value="
          p.duration ? Math.min(1000, (p.current / p.duration) * 1000) : 0
        "
        :max="1000"
        :disabled="
          !p.duration || ['loading', 'unavailable', 'error'].includes(p.status)
        "
        activeColor="#d5e98b"
        backgroundColor="#59614c"
        :block-size="12"
        @change="player.seek(($event.detail.value / 1000) * p.duration)"
      /><text>{{ formatTime(p.duration) }}</text></view
    ><view class="player-status"
      ><text>{{ p.message }}</text
      ><button
        v-if="['error', 'unavailable'].includes(p.status)"
        @click="external"
      >
        去网易云 ↗
      </button></view
    ></view
  >
</template>
<style scoped>
.mini-player {
  position: fixed;
  left: 50%;
  transform: translateX(-50%);
  bottom: calc(14px + env(safe-area-inset-bottom));
  width: min(740px, calc(100% - 36px));
  background: #30372c;
  color: #f3f5e9;
  border: 1px solid #536145;
  border-radius: 12px;
  z-index: 12;
  padding: 12px 15px 10px;
  box-shadow: 0 8px 30px #20291833;
}
.player-top {
  display: flex;
  gap: 12px;
  align-items: center;
}
.player-cover {
  height: 38px;
  width: 38px;
  border-radius: 6px;
  flex-shrink: 0;
}
.player-placeholder {
  display: flex;
  align-items: center;
  justify-content: center;
  background: #536145;
  color: #d5e98b;
  font-size: 23px;
}
.player-info {
  flex: 1;
  min-width: 0;
  cursor: pointer;
}
.player-title {
  font-size: 12px;
  font-weight: 650;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.trial-tag {
  font-size: 8px;
  margin-left: 8px;
  border: 1px solid #cbb76c;
  color: #e9d991;
  padding: 1px 4px;
  border-radius: 3px;
}
.player-artist {
  font-size: 9px;
  color: #a7b297;
  display: block;
  margin-top: 5px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.player-controls {
  display: flex;
  align-items: center;
  gap: 14px;
}
.player-controls button {
  font-size: 17px;
  color: #d8e4c0;
  cursor: pointer;
}
.player-controls .player-toggle {
  display: flex;
  align-items: center;
  justify-content: center;
  border-radius: 50%;
  background: #d5e98b;
  color: #3c4a29;
  width: 32px;
  height: 32px;
  font-size: 15px;
}
.player-close {
  font-size: 23px;
  color: #97a488;
  margin-left: 6px;
  cursor: pointer;
}
.player-progress {
  display: flex;
  align-items: center;
  gap: 10px;
  margin-top: 5px;
}
.player-progress > text {
  font-size: 8px;
  color: #a7b297;
  min-width: 24px;
}
.player-progress slider {
  flex: 1;
  margin: 5px 0;
}
.player-status {
  display: flex;
  gap: 12px;
  align-items: center;
  justify-content: space-between;
  font-size: 8px;
  color: #a6b193;
  min-height: 14px;
  line-height: 1.5;
}
.player-status > text {
  flex: 1;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.player-status button {
  font-size: 9px;
  color: #d5e98b;
  white-space: nowrap;
}
@media (max-width: 760px) {
  .mini-player {
    padding: 11px 12px 9px;
  }
  .player-top {
    gap: 9px;
  }
  .player-controls {
    gap: 10px;
  }
  .player-cover {
    width: 34px;
    height: 34px;
  }
  .player-title {
    font-size: 11px;
  }
  .player-controls button {
    font-size: 15px;
  }
  .player-close {
    margin-left: 0;
  }
  .player-controls .player-toggle {
    width: 29px;
    height: 29px;
  }
}
</style>
