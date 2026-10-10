<script setup lang="ts">
import { ref } from "vue";
const frameKey = ref(0);
</script>

<template>
  <view class="canteen">
    <view class="canteen-heading">
      <view>
        <text class="canteen-title">音乐食堂</text>
        <text class="canteen-note">精神食粮 · 搜首歌，让耳朵先下班。</text>
      </view>
      <!-- #ifdef H5 -->
      <button class="reload" @click="frameKey++">重新加载</button>
      <!-- #endif -->
    </view>
    <!-- #ifdef H5 -->
    <iframe
      :key="frameKey"
      class="music-frame"
      src="https://www.qqmp3.vip/"
      title="音乐食堂 · 精神食粮播放器"
      sandbox="allow-scripts allow-same-origin allow-forms"
      allow="autoplay"
      referrerpolicy="no-referrer"
    ></iframe>
    <text class="canteen-note footer">精神食粮不限量供应，吃饱了再假装热爱工作。</text>
    <!-- #endif -->
    <!-- #ifndef H5 -->
    <text class="canteen-note">音乐食堂目前支持网页试玩版，请在浏览器打开食堂使用。</text>
    <!-- #endif -->
  </view>
</template>

<style scoped>
.canteen { overflow: hidden; border: 1px solid #ded7e6; border-radius: 12px; background: #faf9f2; }
.canteen-heading { display: flex; align-items: center; justify-content: space-between; gap: 12px; padding: 18px; }
.canteen-title { display: block; margin-bottom: 6px; font-size: 18px; font-weight: 600; color: #745693; }
.canteen-note { display: block; font-size: 12px; line-height: 1.8; color: #7e826e; }
.reload { flex-shrink: 0; margin: 0; padding: 8px 12px; border: 1px solid #d9cce6; border-radius: 6px; background: #eee6f4; color: #745693; font-size: 12px; cursor: pointer; }
.music-frame { display: block; width: 100%; height: 76vh; min-height: 560px; max-height: 900px; border: 0; background: #181824; }
.footer { padding: 12px 18px; }
@media (max-width: 760px) {
  .canteen-heading { padding: 14px; }
  .music-frame { height: 72vh; min-height: 480px; }
}
</style>
