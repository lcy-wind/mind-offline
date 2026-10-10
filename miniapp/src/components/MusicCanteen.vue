<script setup lang="ts">
import { ref, onMounted, onUnmounted } from "vue";
const enabled = ref(true),
  frameKey = ref(0),
  loading = ref(true),
  slow = ref(false);
let timer: ReturnType<typeof setTimeout> | undefined;
function startTimer() {
  clearTimeout(timer);
  loading.value = true;
  slow.value = false;
  timer = setTimeout(() => {
    slow.value = true;
  }, 15000);
}
function reload() {
  enabled.value = true;
  frameKey.value++;
  startTimer();
}
function loaded() {
  clearTimeout(timer);
  loading.value = false;
  slow.value = false;
}
function stop() {
  enabled.value = false;
  clearTimeout(timer);
  loading.value = false;
  slow.value = false;
}
onMounted(startTimer);
onUnmounted(() => clearTimeout(timer));
</script>
<template>
  <view class="music-canteen"
    ><view class="canteen-heading"
      ><view
        ><text class="eyebrow">MUSIC CANTEEN / 在这里，听一会儿</text
        ><view class="section-title">音乐食堂</view
        ><text class="muted">在下方播放器里搜索歌曲，留在食堂里听。</text></view
      ><view class="canteen-symbol">♫</view></view
    >
    <view class="embed-toolbar"
      ><view
        ><text class="source-name">铜钟网页播放器</text
        ><text class="source-domain">来源：tonzhon.com · 第三方页面</text></view
      ><view class="embed-controls"
        ><button class="outline" @click="reload">
          {{ enabled ? "重新载入" : "打开播放器" }}</button
        ><button v-if="enabled" class="outline" @click="stop">
          停止并关闭
        </button></view
      ></view
    >
    <!-- #ifdef H5 -->
    <view v-if="enabled" class="embed-container"
      ><text v-if="loading" class="embed-loading">{{
        slow ? "外站响应较慢，可点击“重新载入”重试。" : "正在载入播放器…"
      }}</text
      ><iframe
        :key="frameKey"
        class="external-player"
        src="https://tonzhon.com/"
        title="音乐食堂 · 铜钟网页播放器"
        sandbox="allow-scripts allow-same-origin allow-forms"
        allow="autoplay; encrypted-media; fullscreen"
        referrerpolicy="no-referrer"
        @load="loaded"
      ></iframe
    ></view>
    <view v-else class="embed-closed"
      ><text>♫</text><view>播放器已关闭</view
      ><text class="muted">点“打开播放器”可以重新进入。</text></view
    >
    <!-- #endif -->
    <!-- #ifndef H5 -->
    <view class="embed-closed"
      ><text>♫</text><view>请在食堂网页版体验音乐食堂</view
      ><text class="muted">当前小程序版本不支持嵌入这个外部播放器。</text></view
    >
    <!-- #endif -->
    <view class="embed-note"
      >搜索和播放由原站提供，歌曲可用性、口令及其他限制以原站显示为准。切换到其他食堂栏目或退出账号会关闭此播放器；原有网易云、酷狗仍在“音乐实验室”。</view
    >
  </view>
</template>
<style scoped>
.canteen-heading {
  display: flex;
  justify-content: space-between;
  align-items: center;
  background: #e7e0f0;
  border: 1px solid #d9cde6;
  border-radius: 9px;
  padding: 25px 28px;
  margin: 8px 0 22px;
}
.canteen-heading .section-title {
  font-size: 28px;
  margin: 12px 0;
}
.canteen-heading .muted {
  font-size: 12px;
}
.canteen-symbol {
  font-size: 68px;
  line-height: 1;
  color: #9275b1;
  transform: rotate(-12deg);
  padding: 5px 12px;
}
.embed-toolbar {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 15px;
  margin: 16px 0;
}
.source-name {
  display: block;
  font-size: 13px;
  font-weight: 600;
  color: #737b65;
}
.source-domain {
  display: block;
  font-size: 9px;
  color: #a3a991;
  margin-top: 6px;
}
.embed-controls {
  display: flex;
  gap: 8px;
  flex-shrink: 0;
}
.embed-controls .outline {
  font-size: 10px;
}
.embed-container {
  border: 1px solid #d4dbc5;
  border-radius: 8px;
  overflow: hidden;
  background: #24272d;
  position: relative;
}
.external-player {
  display: block;
  width: 100%;
  height: 76vh;
  min-height: 620px;
  border: 0;
  background: #24272d;
}
.embed-loading {
  display: block;
  padding: 12px 16px;
  font-size: 11px;
  line-height: 1.7;
  background: #f0ede3;
  color: #8d8878;
}
.embed-closed {
  border: 1px dashed #cbd4b8;
  border-radius: 8px;
  padding: 65px 20px;
  text-align: center;
  color: #8a9973;
  line-height: 2.4;
  background: #f8f8ed;
  font-size: 14px;
}
.embed-closed > text:first-child {
  font-size: 52px;
  display: block;
}
.embed-note {
  font-size: 10px;
  color: #a0a78f;
  line-height: 1.9;
  margin: 17px 0 30px;
}
@media (max-width: 760px) {
  .canteen-heading {
    padding: 22px 18px;
    margin-top: 0;
  }
  .canteen-heading .section-title {
    font-size: 24px;
  }
  .canteen-heading .eyebrow {
    font-size: 7px;
  }
  .canteen-heading .muted {
    font-size: 10px;
  }
  .canteen-symbol {
    font-size: 46px;
    padding: 0;
  }
  .embed-toolbar {
    gap: 9px;
    flex-wrap: wrap;
  }
  .embed-controls .outline {
    font-size: 9px;
    padding: 8px;
  }
  .source-name {
    font-size: 12px;
  }
  .source-domain {
    font-size: 8px;
  }
  .external-player {
    height: 75vh;
    min-height: 560px;
  }
  .embed-note {
    font-size: 9px;
  }
  .embed-loading {
    font-size: 10px;
    padding: 11px;
  }
}
</style>
