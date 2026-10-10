<script setup lang="ts">
import { ref, computed, onMounted, onBeforeUnmount, nextTick } from "vue";
import { request, ApiError } from "../lib/api";
import { streamHealing, type ChatEvent } from "../lib/healing-stream";
// #ifdef MP-WEIXIN
import { nativeHealing } from "../lib/healing-native";
// #endif
interface Role { code: string; name: string; style: string }
interface Conversation { id: string; title: string; mbti: string; name: string; style: string; updated_at: string }
interface Turn { id: string; user: string; assistant: string; status: string; created_at: string }
const emit = defineEmits<{ (e: "auth-expired"): void }>();
const roles = ref<Role[]>([]), conversations = ref<Conversation[]>([]), turns = ref<Turn[]>([]);
const active = ref<Conversation | null>(null);
const enabled = ref(false), ready = ref(false), loading = ref(false), creating = ref(false), sending = ref(false);
const error = ref(""), draft = ref(""), roleCode = ref("INFP"), roleName = ref(""), roleStyle = ref("");
const showCreate = ref(false), deleting = ref("");
const messages = ref<HTMLDivElement | null>(null);
const composer = ref<HTMLTextAreaElement | null>(null);
const follow = ref(true);
const pickedRole = computed(() => roles.value.find(r => r.code === roleCode.value));
let alive = true, revision = 0;
let controller: {abort(): void} | null = null;
const messageScrollTop = ref(0);
let stopped = false;
function handleError(e: unknown) {
  if (e instanceof ApiError && e.status === 401) emit("auth-expired");
  error.value = e instanceof Error ? e.message : "聊天暂时没跟上，请稍后重试";
}
async function list() {
  const data = await request<{conversations: Conversation[]}>("/healing/conversations");
  if (alive) conversations.value = data.conversations;
}
async function init() {
  error.value = ""; loading.value = true;
  try {
    const config = await request<{enabled: boolean; roles: Role[]}>("/healing/config");
    if (!alive) return;
    roles.value = config.roles; enabled.value = config.enabled;
    await list(); if (!alive) return;
    ready.value = true;
    if (conversations.value.length) await openConversation(conversations.value[0]);
    else showCreate.value = true;
  } catch (e) { if (alive) handleError(e); }
  finally { if (alive) loading.value = false; }
}
function stop() { stopped = true; controller?.abort(); }
async function scrollBottom() {
  await nextTick();
  if (!alive || !follow.value) return;
  // #ifdef H5
  if (messages.value) messages.value.scrollTop = messages.value.scrollHeight;
  // #endif
  // #ifdef MP-WEIXIN
  messageScrollTop.value += 100000;
  // #endif
}
async function openConversation(c: Conversation) {
  if (sending.value) return;
  const run = ++revision;
  active.value = c; turns.value = []; draft.value = ""; showCreate.value = false; loading.value = true; error.value = "";
  try {
    const data = await request<{conversation: Conversation; turns: Turn[]}>("/healing/conversations/" + c.id);
    if (!alive || run !== revision) return;
    active.value = data.conversation; turns.value = data.turns; follow.value = true; scrollBottom();
  } catch (e) { if (alive && run === revision) handleError(e); }
  finally { if (alive && run === revision) loading.value = false; }
}
function newConversation() { if (sending.value) return; revision++; showCreate.value = true; active.value = null; turns.value = []; error.value = ""; draft.value = ""; }
async function createConversation() {
  if (creating.value || !ready.value) return;
  creating.value = true; error.value = "";
  try {
    const c = await request<Conversation>("/healing/conversations", "POST", {mbti:roleCode.value,name:roleName.value.trim(),style:roleStyle.value.trim()});
    if (!alive) return;
    conversations.value.unshift(c); await openConversation(c);
  } catch (e) { if (alive) handleError(e); }
  finally { if (alive) creating.value = false; }
}
function removeConversation(c: Conversation) {
  if (sending.value || deleting.value) return;
  uni.showModal({title:"删除这段聊天？",content:"聊天记录删除后无法恢复，其他会话不受影响。",confirmText:"删除",success: async result => {
    if (!result.confirm || !alive) return;
    deleting.value = c.id;
    try {
      await request("/healing/conversations/" + c.id + "/delete", "POST", {});
      if (!alive) return;
      conversations.value = conversations.value.filter(v => v.id !== c.id);
      if (active.value?.id === c.id) newConversation();
    } catch (e) { if (alive) handleError(e); }
    finally { if (alive) deleting.value = ""; }
  }});
}
async function send(existing?: Turn) {
  if (!active.value || sending.value || loading.value || !enabled.value) return;
  const text = existing?.user || draft.value.trim();
  if (!text) return;
  if ([...text].length > 4000) { error.value = "这条消息有点长，请控制在 4000 字以内。"; return; }
  const id = active.value.id, run = revision;
  const requestID = existing?.id || Date.now().toString(36) + "-" + Math.random().toString(36).slice(2) + "-" + Math.random().toString(36).slice(2);
  let turn = existing;
  if (turn) { turn.assistant = ""; turn.status = "pending"; }
  else { turns.value.push({id:requestID,user:text,assistant:"",status:"pending",created_at:new Date().toISOString()}); turn = turns.value[turns.value.length-1]; draft.value = ""; }
  const target = turn;
  sending.value = true; error.value = ""; stopped = false; follow.value = true; scrollBottom();
  // #ifdef H5
  if (!existing) composer.value?.focus({preventScroll: true});
  // #endif
  const accept = (e: ChatEvent) => {
    if (!alive || run !== revision || active.value?.id !== id) return;
    if (e.event === "delta") { target.assistant += e.data.text; scrollBottom(); }
    if (e.event === "done") { target.assistant = e.data.assistant; target.status = e.data.status; scrollBottom(); }
  };
  let pending: {promise: Promise<void>; abort(): void};
  // #ifdef H5
  const webAbort = new AbortController();
  pending = {promise: streamHealing(id,requestID,text,webAbort.signal,accept), abort: () => webAbort.abort()};
  // #endif
  // #ifdef MP-WEIXIN
  pending = nativeHealing(id,requestID,text,accept);
  // #endif
  const abort = pending!; controller = abort;
  const timeout = setTimeout(() => abort.abort(), 100000);
  try {
    await pending!.promise;
    if (!alive || run !== revision) return;
    await list();
    const updated = conversations.value.find(c => c.id === id); if (updated) active.value = updated;
  } catch (e) {
    if (!alive || run !== revision) return;
    target.status = stopped ? "interrupted" : "failed";
    if (!stopped) handleError((e as Error).name === "AbortError" ? new Error("回复等待较久，已停止。可以稍后重试。") : e);
  } finally {
    clearTimeout(timeout);
    if (controller === abort) controller = null;
    if (alive && run === revision) sending.value = false;
  }
}
function editDraft(e: Event) { draft.value = (e.target as HTMLTextAreaElement).value; }
function keyboardSend(e: KeyboardEvent) {
  if (e.key === "Enter" && !e.shiftKey && !e.isComposing) { e.preventDefault(); send(); }
}
function nativeScroll(e: any) {
  // #ifdef MP-WEIXIN
  follow.value = e.detail.scrollHeight - e.detail.scrollTop - 400 < 90;
  // #endif
}
function trackScroll() {
  const box = messages.value;
  if (box) follow.value = box.scrollHeight - box.scrollTop - box.clientHeight < 90;
}
onMounted(init);
onBeforeUnmount(() => { alive = false; revision++; stop(); });
defineExpose({stop});
</script>

<template>
  <view class="healing">
    <view class="healing-hero"><text class="eyebrow">MIND OFFLINE / A LITTLE ROOM TO BREATHE</text><text class="hero-title">精神疗愈</text><text class="hero-copy">今天不解决全世界，先聊聊你。</text><text class="hero-note">MBTI 是角色风格设定。这里是 AI 陪聊，不是专业心理咨询。</text></view>
    <view v-if="error" class="chat-error" role="alert">{{ error }}<button v-if="!ready" @click="init">重新连接</button></view>
    <view v-if="ready && !enabled" class="chat-error">聊天服务正在准备中，请稍后再来。</view>
    <view class="chat-layout">
      <view class="sessions">
        <button class="new-chat" :disabled="sending || creating || loading" @click="newConversation">＋ 开一段新聊天</button>
        <text class="session-caption">我的聊天 · 仅当前账号可见</text>
        <view v-if="!conversations.length" class="session-empty">给今天留一点说话的空间。</view>
        <view v-for="c in conversations" :key="c.id" class="session-row" :class="{ selected: active?.id === c.id }">
          <button class="session-open" :disabled="sending || creating || loading" @click="openConversation(c)"><text class="session-title">{{ c.title }}</text><text class="session-role">{{ c.mbti }} · {{ c.name }}</text></button>
          <button class="session-delete" :disabled="sending || !!deleting" @click="removeConversation(c)" :aria-label="'删除聊天 ' + c.title">×</button>
        </view>
      </view>
      <view class="chat-main">
        <view v-if="showCreate" class="role-maker">
          <text class="section-title">找个合拍的聊天搭子</text><text class="subtle">选一种气质，再给它一点你的设定。</text>
          <view class="role-grid"><button v-for="role in roles" :key="role.code" :class="{ picked: roleCode === role.code }" @click="roleCode = role.code"><text class="role-code">{{ role.code }}</text><text>{{ role.name }}</text></button></view>
          <text class="role-description">{{ pickedRole?.style }}</text>
          <text class="field-label">怎么称呼 TA（可选）</text><input v-model="roleName" maxlength="40" :placeholder="pickedRole?.name || '给角色起个名字'" class="chat-input" />
          <text class="field-label">角色背景和聊天偏好（可选）</text><textarea v-model="roleStyle" maxlength="500" placeholder="例如：喜欢电影和散步，语气温柔但有主见，可以接梗，不要一上来就讲道理。" class="style-input" />
          <button class="primary" :disabled="creating || !enabled || !ready" @click="createConversation">{{ creating ? '正在给你们留座…' : '就和 TA 聊聊 →' }}</button>
        </view>
        <template v-else-if="active">
          <view class="chat-heading"><view><text class="chat-name">{{ active.name }}</text><text class="role-tag">{{ active.mbti }} · AI 聊天搭子</text></view><button :disabled="sending" class="refresh-chat" @click="openConversation(active)">刷新记录</button></view>
          <scroll-view scroll-y :scroll-top="messageScrollTop" class="message-scroll" @scroll="nativeScroll"><div ref="messages" class="messages" @scroll="trackScroll">
            <view v-if="loading" class="chat-empty">正在翻开这段聊天…</view>
            <view v-else-if="!turns.length" class="chat-empty"><text class="empty-icon">✳</text><text>这里没有标准答案。</text><text class="subtle">从一句“今天好累”，或者一个奇怪的脑洞开始吧。</text></view>
            <view v-for="turn in turns" :key="turn.id" class="turn">
              <view class="message user"><text class="message-label">你</text><text class="bubble">{{ turn.user }}</text></view>
              <view class="message assistant"><text class="message-label">{{ active.name }}</text><text class="bubble">{{ turn.assistant || (turn.status === 'pending' ? '正在组织语言…' : '这条回复还没完成。') }}<text v-if="turn.status === 'pending' && sending" class="typing-dot"> ▍</text></text>
                <view v-if="turn.status !== 'complete'" class="turn-state"><text>{{ turn.status === 'pending' ? (sending ? '回复中…' : '上一条回复尚未完成，可刷新查看') : turn.status === 'interrupted' ? '已停止生成' : '回复未完成' }}</text><button v-if="turn.status !== 'pending'" :disabled="sending" @click="send(turn)">重试这条</button></view>
              </view>
            </view>
          </div></scroll-view>
          <button v-if="!follow && sending" class="follow-button" @click="follow = true; scrollBottom()">回到最新回复 ↓</button>
          <view class="composer">
            <!-- #ifdef H5 -->
            <component :is="'textarea'" ref="composer" class="composer-input" :value="draft" @input="editDraft" maxlength="4000" rows="3" :disabled="loading || !enabled" placeholder="说说今天的心情，或者随便聊点什么…" aria-label="聊天消息" @keydown="keyboardSend" />
            <!-- #endif -->
            <!-- #ifdef MP-WEIXIN -->
            <textarea v-model="draft" class="composer-input" :maxlength="4000" :disabled="loading || !enabled" :fixed="true" :show-confirm-bar="false" :adjust-position="true" :cursor-spacing="24" placeholder="说说今天的心情，或者随便聊点什么…" />
            <!-- #endif -->
            <view class="composer-bottom"><text>{{ draft.length }}/4000<!-- #ifdef H5 --> · Enter 发送，Shift+Enter 换行<!-- #endif --></text><button v-if="sending" @click="stop" class="stop-button">停止生成 ■</button><button v-else class="primary send-button" :disabled="!draft.trim() || loading || !enabled" @click="send()">发送 ↑</button></view>
          </view>
          <text class="privacy-note">回复由 AI 生成。聊天内容与角色设定会发送至智谱处理；记录保存在你的食堂账号下。</text>
        </template>
        <view v-else class="chat-empty">{{ loading ? '正在准备聊天空间…' : '选择一段聊天，或新建一个角色。' }}</view>
      </view>
    </view>

  </view>
</template>

<style scoped>
.healing { border: 1px solid #e0d7e7; border-radius: 12px; overflow: hidden; background: #faf9f2; }
.healing-hero { padding: 26px; background: #ece4f3; }
.eyebrow { display: block; font-size: 10px; letter-spacing: 1.7px; color: #a18bb0; }
.hero-title { display: block; font-size: 30px; font-weight: 700; color: #6e4e84; margin: 12px 0 8px; }
.hero-copy { display: block; font-size: 15px; color: #8b709a; }
.hero-note { display: block; margin-top: 14px; font-size: 10px; line-height: 1.7; color: #aa99b2; }
.chat-layout { display: flex; min-height: 610px; }
.sessions { flex: 0 0 210px; border-right: 1px solid #e8e2ed; padding: 16px 12px; box-sizing: border-box; max-height: 850px; overflow-y: auto; }
.new-chat { width: 100%; margin: 0 0 15px; padding: 12px 8px; background: #eae1f2; color: #795990; border: 1px solid #d8c8e2; border-radius: 7px; font-size: 12px; }
.session-caption { display: block; font-size: 10px; color: #a59aaf; margin: 0 8px 12px; }
.session-empty { padding: 16px 8px; color: #b3a8ba; font-size: 11px; line-height: 1.8; }
.session-row { display: flex; align-items: center; margin: 4px 0; border-radius: 6px; }
.session-row.selected { background: #f0e9f5; }
.session-open { flex: 1; min-width: 0; text-align: left; margin: 0; padding: 11px 9px; background: transparent; border: 0; line-height: 1.6; }
.session-title { display: block; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; font-size: 12px; color: #7d698a; }
.session-role { display: block; font-size: 10px; color: #b09ebc; }
.session-delete { flex-shrink: 0; margin: 0; padding: 4px 8px; background: transparent; border: 0; color: #b9aaba; font-size: 16px; }
.chat-main { flex: 1; min-width: 0; position: relative; }
.role-maker { padding: 25px; }
.section-title { display: block; color: #775687; font-size: 20px; font-weight: 600; margin-bottom: 8px; }
.subtle { display: block; color: #ad9fb2; font-size: 12px; line-height: 1.9; }
.role-grid { display: grid; grid-template-columns: repeat(4,minmax(0,1fr)); gap: 8px; margin: 20px 0 12px; }
.role-grid button { display: flex; flex-direction: column; gap: 4px; padding: 10px 5px; margin: 0; border: 1px solid #e4dce9; border-radius: 7px; background: #faf8fc; color: #a48ab2; font-size: 10px; line-height: 1.5; }
.role-grid button.picked { background: #e8dcf0; border-color: #ae8fc2; color: #765189; }
.role-code { font-size: 14px; font-weight: 600; }
.role-description { display: block; min-height: 38px; color: #96829f; font-size: 12px; line-height: 1.8; }
.field-label { display: block; margin: 18px 0 8px; font-size: 12px; color: #8c759b; }
.chat-input, .style-input { box-sizing: border-box; width: 100%; background: white; border: 1px solid #e3daea; border-radius: 7px; padding: 12px; font-size: 12px; color: #73627d; }
.chat-input { height: 42px; }.style-input { height: 86px; }
.primary { background: #85639a; color: white; border: 0; border-radius: 7px; padding: 11px 18px; margin: 20px 0 0; font-size: 13px; }
.chat-heading { display: flex; align-items: center; justify-content: space-between; padding: 18px 22px; border-bottom: 1px solid #e9e1ed; }
.chat-name { display: block; font-size: 17px; font-weight: 600; color: #79548c; }.role-tag { display: block; margin-top: 5px; font-size: 10px; color: #ad99b8; }
.refresh-chat { padding: 5px 8px; margin: 0; border: 0; background: transparent; font-size: 10px; color: #aa93b6; }
.messages { height: 440px; overflow-y: auto; overscroll-behavior: contain; padding: 20px 24px; box-sizing: border-box; scrollbar-width: thin; scrollbar-color: #d9cbe1 transparent; }
.chat-empty { padding: 60px 20px; text-align: center; font-size: 14px; color: #a591b1; line-height: 2; }.chat-empty text { display: block; }.empty-icon { font-size: 40px; color: #bba4ca; margin-bottom: 15px; }
.message { display: flex; flex-direction: column; align-items: flex-start; margin-bottom: 20px; }.message.user { align-items: flex-end; }
.message-label { font-size: 10px; color: #b09bb8; margin-bottom: 6px; }
.bubble { display: block; max-width: 90%; white-space: pre-wrap; overflow-wrap: anywhere; font-size: 14px; line-height: 1.9; padding: 12px 16px; border-radius: 12px 12px 12px 3px; background: #f1edf5; color: #75617f; }
.user .bubble { background: #e9efcf; color: #737c51; border-radius: 12px 12px 3px 12px; }
.typing-dot { color: #b599c6; }.turn-state { display: flex; align-items: center; gap: 8px; font-size: 10px; color: #b49aac; margin-top: 7px; }.turn-state button { border: 0; background: transparent; margin: 0; padding: 2px 5px; font-size: 10px; color: #8b699d; }
.composer { margin: 0 22px 10px; border: 1px solid #ddcfe6; border-radius: 10px; padding: 12px; background: white; }
.composer:focus-within { border-color: #b49ac5; box-shadow: 0 0 0 2px #eae0f240; }
.composer-input { display: block; box-sizing: border-box; -webkit-appearance: none; appearance: none; width: 100%; min-width: 0; height: 84px; min-height: 84px; max-height: 170px; resize: none; overflow-y: auto; outline: none; border: none; border-radius: 0; box-shadow: none; margin: 0 0 10px; padding: 0; color: #6f5a7c; background: transparent; font-family: inherit; font-size: 13px; line-height: 1.8; }
.composer-input::placeholder { color: #b6a8bc; }
.composer-input:disabled { color: #a799af; cursor: default; }
.composer-bottom { display: flex; align-items: center; justify-content: space-between; gap: 10px; }.composer-bottom > text { font-size: 9px; color: #b6a8bc; }.send-button { margin: 0; padding: 8px 16px; font-size: 12px; }.stop-button { margin: 0; padding: 8px 12px; font-size: 12px; border: 1px solid #d9c7e2; border-radius: 6px; background: #f2eaf7; color: #9675a5; }
.privacy-note { display: block; padding: 0 22px 16px; font-size: 9px; color: #b4a6b8; line-height: 1.8; }
.chat-error { padding: 12px 22px; font-size: 12px; color: #ac778b; background: #faeef0; line-height: 1.8; }.chat-error button { display: inline; background: none; border: 0; font-size: 11px; color: #97677e; }
.follow-button { position: absolute; right: 25px; bottom: 180px; margin: 0; font-size: 11px; padding: 7px 12px; background: #eee1f6; border: 1px solid #dac8e6; border-radius: 20px; color: #876396; }
button:disabled { opacity: .45; }button::after { border: 0; }
@media (max-width: 960px) { .sessions { flex-basis: 165px; } .role-grid { grid-template-columns: repeat(2,minmax(0,1fr)); } }
@media (max-width: 760px) { .healing-hero { padding: 22px 16px; }.hero-title { font-size: 26px; }.chat-layout { flex-direction: column; }.sessions { flex-basis: auto; max-height: 190px; border-right: 0; border-bottom: 1px solid #e8e2ed; }.new-chat { margin-bottom: 8px; }.role-maker { padding: 20px 15px; }.role-grid { grid-template-columns: repeat(4,minmax(0,1fr)); gap: 5px; }.role-grid button { font-size: 8px; }.role-code { font-size: 12px; }.messages { padding: 16px 12px; height: 400px; }.bubble { max-width: 94%; font-size: 13px; padding: 10px 13px; }.chat-heading { padding: 15px; }.composer { margin: 0 12px 10px; }.composer-bottom > text { max-width: 170px; line-height: 1.6; }.privacy-note { padding: 0 12px 15px; } }
/* #ifdef MP-WEIXIN */
.message-scroll { height: 400px; }
.messages { height: auto; min-height: 400px; overflow: visible; box-sizing: border-box; }
/* #endif */
</style>
