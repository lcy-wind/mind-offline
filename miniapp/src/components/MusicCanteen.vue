<template>
  <view class="canteen-empty">
    <text class="canteen-icon">♫</text>
    <text class="canteen-title">音乐食堂 · 音源待定</text>
    <text class="canteen-copy">耳朵先下班，好音乐慢慢找。</text>
    <text class="canteen-note">暂未接入音源，可切换至网易云或酷狗使用已绑定的账号。</text>
  </view>
</template>

<style scoped>
.canteen-empty {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 14px;
  padding: 48px 20px;
  text-align: center;
  border: 1px solid #ded7e6;
  border-radius: 12px;
  background: #faf9f2;
}
.canteen-icon { font-size: 36px; color: #9471b0; }
.canteen-title { font-size: 18px; font-weight: 600; color: #745693; }
.canteen-copy { font-size: 14px; color: #7e826e; }
.canteen-note { font-size: 12px; line-height: 1.8; color: #929681; }
</style>
