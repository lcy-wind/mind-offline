<script setup lang="ts">
import { ref, computed, watch, onMounted, onUnmounted } from "vue";
import { onShow, onHide, onUnload } from "@dcloudio/uni-app";
import MusicPanel from "../../components/MusicPanel.vue";
import MiniPlayer from "../../components/MiniPlayer.vue";
import { player, playerState, checkPlayerBinding } from "../../lib/player";
const musicForeground = ref(true);
import {
  request,
  currentToken,
  SESSION_KEY,
  ApiError,
  type Dish,
  type Guest,
  type Line,
  type Order,
} from "../../lib/api";
const tab = ref("menu"),
  category = ref("全部补给"),
  menu = ref<Dish[]>([]),
  me = ref<Guest>({ id: "", username: "", balance: 0, claimed_today: false }),
  orders = ref<Order[]>([]);
const loading = ref(true),
  error = ref(""),
  busy = ref(false),
  selected = ref<Dish | null>(null),
  quantity = ref(1),
  mood = ref("灵魂离线"),
  note = ref(""),
  cartOpen = ref(false),
  receipt = ref<Order | null>(null);
const moods = ["勉强清醒", "灵魂离线", "已读乱回"];
const categories = ["全部补给", "续命主食", "精神饮品", "摸鱼小食", "离职套餐"];
const cart = ref<Line[]>([]);
watch(
  cart,
  (v) => {
    if (me.value.id) uni.setStorageSync("mind-offline-cart:" + me.value.id, v);
  },
  { deep: true, flush: "sync" },
);
const authMode = ref<"login" | "register">("login"),
  username = ref(""),
  password = ref(""),
  confirmPassword = ref(""),
  authError = ref(""),
  bindLegacy = ref(false);
const hasLegacy = ref(Boolean(uni.getStorageSync("mind-offline-token")));
let displayedToken = currentToken();
function clearPrivateState() {
  player.reset();
  me.value = { id: "", username: "", balance: 0, claimed_today: false };
  cart.value = [];
  orders.value = [];
  receipt.value = null;
  selected.value = null;
  cartOpen.value = false;
  note.value = "";
  tab.value = "menu";
}
function ensureSession() {
  if (!currentToken() || currentToken() !== displayedToken || !me.value.id) {
    clearPrivateState();
    displayedToken = currentToken();
    refresh();
    return false;
  }
  return true;
}
// #ifdef H5
function syncTabs(event: StorageEvent) {
  if (event.key === SESSION_KEY || event.key === null) {
    clearPrivateState();
    displayedToken = currentToken();
    refresh();
  }
}
onMounted(() => window.addEventListener("storage", syncTabs));
onUnmounted(() => window.removeEventListener("storage", syncTabs));
// #endif
function forgetSession() {
  uni.removeStorageSync(SESSION_KEY);
  displayedToken = "";
  clearPrivateState();
}
function handleFailure(e: unknown) {
  if (e instanceof ApiError && e.status === 401) {
    forgetSession();
    authError.value = "登录已过期，请重新登录";
  } else if (!(e instanceof ApiError && e.status === 0))
    error.value = (e as Error).message;
}
async function authenticate() {
  if (busy.value) return;
  authError.value = "";
  if (
    authMode.value === "register" &&
    password.value !== confirmPassword.value
  ) {
    authError.value = "两次输入的密码不一致";
    return;
  }
  busy.value = true;
  try {
    const data: Record<string, unknown> = {
      username: username.value,
      password: password.value,
    };
    if (authMode.value === "register" && bindLegacy.value)
      data.legacy_token = uni.getStorageSync("mind-offline-token");
    const session = await request<{ token: string }>(
      "/auth/" + authMode.value,
      "POST",
      data,
    );
    clearPrivateState();
    uni.setStorageSync(SESSION_KEY, session.token);
    displayedToken = session.token;
    if (data.legacy_token) {
      uni.removeStorageSync("mind-offline-token");
      hasLegacy.value = false;
      bindLegacy.value = false;
    }
    password.value = "";
    confirmPassword.value = "";
    error.value = "";
    await refresh();
  } catch (e) {
    authError.value = (e as Error).message;
  } finally {
    busy.value = false;
  }
}
async function signOut() {
  if (!ensureSession()) return;
  if (busy.value) return;
  busy.value = true;
  try {
    await request("/auth/logout", "POST");
    forgetSession();
    authError.value = "";
    error.value = "";
  } catch (e) {
    if (e instanceof ApiError && e.status === 401) forgetSession();
    else toast((e as Error).message);
  } finally {
    busy.value = false;
  }
}
const filtered = computed(() =>
  menu.value.filter(
    (d) => category.value === "全部补给" || d.category === category.value,
  ),
);
const count = computed(() => cart.value.reduce((s, i) => s + i.quantity, 0));
const total = computed(() =>
  cart.value.reduce((s, i) => s + (i.price || 0) * i.quantity, 0),
);
const active = computed(
  () =>
    orders.value.filter((o) => !["completed", "cancelled"].includes(o.status))
      .length,
);
const labels: Record<string, string> = {
  pending: "店长正在装作没看见",
  cooking: "厨师正在与食材沟通",
  ready: "精神食粮已出锅",
  completed: "今日精神已充值",
  cancelled: "本次离职已撤回",
};
const steps = ["pending", "cooking", "ready", "completed"];
let timer: ReturnType<typeof setInterval> | undefined;
let fetching = false;
function toast(t: string) {
  uni.showToast({ title: t, icon: "none", duration: 2200 });
}
async function refresh() {
  if (fetching) return;
  if (displayedToken !== currentToken()) {
    clearPrivateState();
    displayedToken = currentToken();
  }
  if (!currentToken()) {
    loading.value = false;
    return;
  }
  fetching = true;
  const expected = currentToken();
  try {
    const [g, ds, os] = await Promise.all([
      request<Guest>("/me"),
      request<Dish[]>("/menu"),
      request<Order[]>("/orders"),
    ]);
    if (expected !== currentToken()) return;
    const changed = me.value.id !== g.id;
    player.setOwner(g.id);
    me.value = g;
    void checkPlayerBinding();
    menu.value = ds;
    orders.value = os;
    if (changed)
      cart.value = uni.getStorageSync("mind-offline-cart:" + g.id) || [];
    error.value = "";
  } catch (e) {
    handleFailure(e);
  } finally {
    loading.value = false;
    fetching = false;
  }
}
onShow(() => {
  musicForeground.value = true;
  refresh();
  timer = setInterval(() => {
    if (!busy.value) refresh();
  }, 8000);
});
onHide(() => {
  musicForeground.value = false;
  clearInterval(timer);
});
onUnload(() => {
  clearInterval(timer);
  player.reset();
});
function choose(d: Dish) {
  if (!ensureSession()) return;
  if (!d.available) return;
  selected.value = d;
  quantity.value = 1;
}
function add() {
  if (!ensureSession()) return;
  if (!selected.value) return;
  const d = selected.value;
  const old = cart.value.find(
    (i) => i.dish_id === d.id && i.mood === mood.value,
  );
  if (
    count.value + quantity.value > 30 ||
    (old?.quantity || 0) + quantity.value > 10
  ) {
    toast("一单最多30份，同款同状态最多10份");
    return;
  }
  if (old) old.quantity += quantity.value;
  else
    cart.value.push({
      dish_id: d.id,
      name: d.name,
      emoji: d.emoji,
      price: d.price,
      quantity: quantity.value,
      mood: mood.value,
    });
  selected.value = null;
  toast("已加入精神补给袋");
}
function change(i: number, n: number) {
  if (!ensureSession()) return;
  if (n > 0 && (cart.value[i].quantity >= 10 || count.value >= 30)) return;
  cart.value[i].quantity += n;
  if (cart.value[i].quantity <= 0) cart.value.splice(i, 1);
}
async function claim() {
  if (!ensureSession()) return;
  if (busy.value) return;
  busy.value = true;
  try {
    me.value = await request<Guest>("/claim", "POST");
    toast("今日100精神值已到账");
  } catch (e) {
    handleFailure(e);
    toast((e as Error).message);
  } finally {
    busy.value = false;
  }
}
async function checkout() {
  if (!ensureSession()) return;
  if (busy.value || !cart.value.length) return;
  busy.value = true;
  try {
    const data = {
      mood: mood.value,
      note: note.value,
      items: cart.value.map((i) => ({
        dish_id: i.dish_id,
        quantity: i.quantity,
        mood: i.mood,
      })),
    };
    const signature = JSON.stringify(data);
    let pending = uni.getStorageSync("mind-offline-pending:" + me.value.id);
    if (!pending || pending.signature !== signature) {
      pending = {
        signature,
        key:
          Date.now().toString(36) +
          "-" +
          Math.random().toString(36).slice(2) +
          "-" +
          Math.random().toString(36).slice(2),
      };
      uni.setStorageSync("mind-offline-pending:" + me.value.id, pending);
    }
    const o = await request<Order>("/orders", "POST", {
      ...data,
      request_key: pending.key,
    });
    cart.value = [];
    note.value = "";
    cartOpen.value = false;
    tab.value = "orders";
    receipt.value = o;
    uni.removeStorageSync("mind-offline-pending:" + me.value.id);
    await refresh();
  } catch (e) {
    handleFailure(e);
    toast((e as Error).message);
  } finally {
    busy.value = false;
  }
}
function number(o: Order) {
  return "MO-" + String(o.number).padStart(4, "0");
}
function date(s: string) {
  const d = new Date(s);
  return `${d.getMonth() + 1}/${d.getDate()} ${String(d.getHours()).padStart(2, "0")}:${String(d.getMinutes()).padStart(2, "0")}`;
}
async function saveReceipt() {
  if (!ensureSession()) return;
  const o = receipt.value;
  if (!o) return;
  const ctx = uni.createCanvasContext("receiptCanvas");
  const h = 600;
  ctx.setFillStyle("#f4f1e8");
  ctx.fillRect(0, 0, 360, h);
  ctx.setFillStyle("#6350b9");
  ctx.fillRect(0, 0, 360, 10);
  ctx.setFillStyle("#24271f");
  ctx.setFontSize(28);
  ctx.fillText("精神离职", 32, 65);
  ctx.setFontSize(12);
  ctx.fillText("MIND OFFLINE / 精神补给凭证", 32, 91);
  ctx.setFontSize(15);
  ctx.fillText(number(o) + "    " + date(o.created_at), 32, 132);
  ctx.setFontSize(13);
  let y = 174;
  o.items.slice(0, 7).forEach((i) => {
    ctx.fillText(i.name!.slice(0, 14) + " × " + i.quantity, 32, y);
    ctx.fillText(String(i.price! * i.quantity), 300, y);
    y += 30;
  });
  if (o.items.length > 7) {
    ctx.fillText("…更多补给见订单详情", 32, y);
    y += 30;
  }
  ctx.setFontSize(20);
  ctx.fillText("合计 " + o.total + " 精神值", 32, y + 28);
  ctx.setFontSize(12);
  ctx.fillText("虚拟体验 · 未产生真实支付", 32, y + 52);
  ctx.setFillStyle("#6350b9");
  ctx.setFontSize(14);
  const chars = Array.from(o.quote);
  chars.forEach((_, i) => {
    if (i % 20 === 0)
      ctx.fillText(
        chars.slice(i, i + 20).join(""),
        32,
        470 + Math.floor(i / 20) * 24,
      );
  });
  ctx.setFontSize(11);
  ctx.fillText("人可以不清醒，饭不能不吃。", 32, 560);
  ctx.draw(false, () =>
    uni.canvasToTempFilePath({
      canvasId: "receiptCanvas",
      width: 360,
      height: h,
      destWidth: 720,
      destHeight: 1200,
      success: (res) => {
        // #ifdef H5
        const a = document.createElement("a");
        a.href = res.tempFilePath;
        a.download = number(o) + ".png";
        a.click();
        toast("小票已生成，可在下载中查看");
        // #endif
        // #ifdef MP-WEIXIN
        uni.saveImageToPhotosAlbum({
          filePath: res.tempFilePath,
          success: () => toast("已保存到相册"),
          fail: () => toast("请允许保存到相册后重试"),
        });
        // #endif
      },
      fail: () => toast("保存失败，请稍后重试"),
    }),
  );
}
</script>

<template>
  <view v-if="!me.id" class="auth-page">
    <view class="auth-story"
      ><text class="auth-wordmark">精神离职 / MIND OFFLINE</text
      ><view class="auth-title"
        >把工作放下，<br /><text>把快乐存进账号。</text></view
      ><text class="auth-star">✳</text
      ><text class="auth-subtitle"
        >你的精神值、你的订单、你的摸鱼时刻。</text
      ></view
    >
    <view class="auth-card"
      ><text class="eyebrow">YOUR PERSONAL OFFLINE SPACE</text
      ><view class="section-title">{{
        authMode === "login" ? "欢迎回来，打工人。" : "领取你的离职工牌。"
      }}</view>
      <view class="auth-tabs"
        ><button
          :class="{ picked: authMode === 'login' }"
          :disabled="busy"
          @click="
            authMode = 'login';
            authError = '';
          "
        >
          登录</button
        ><button
          :class="{ picked: authMode === 'register' }"
          :disabled="busy"
          @click="
            authMode = 'register';
            authError = '';
          "
        >
          注册新账号
        </button></view
      >
      <view class="field-label">用户名</view
      ><input
        v-model="username"
        class="auth-input"
        maxlength="24"
        placeholder="3—24位字母、数字或下划线"
        :disabled="busy"
      />
      <view class="field-label">密码</view
      ><input
        v-model="password"
        class="auth-input"
        password
        maxlength="72"
        placeholder="至少8个字符"
        :disabled="busy"
        @confirm="authenticate"
      />
      <template v-if="authMode === 'register'"
        ><view class="field-label">确认密码</view
        ><input
          v-model="confirmPassword"
          class="auth-input"
          password
          maxlength="72"
          placeholder="再输入一次密码"
          :disabled="busy"
          @confirm="authenticate"
        />
        <view
          v-if="hasLegacy"
          class="legacy-option"
          @click="!busy && (bindLegacy = !bindLegacy)"
          ><text
            >{{
              bindLegacy ? "☑" : "☐"
            }}
            将本浏览器的旧订单和余额绑定到新账号</text
          ><text class="muted"
            >仅在确认这些记录属于你时勾选；绑定后不能再次转移。</text
          ></view
        ></template
      >
      <text v-if="authError || error" class="auth-error">{{
        authError || error
      }}</text>
      <button
        class="primary wide"
        :disabled="busy || !username || !password"
        @click="authenticate"
      >
        {{
          busy
            ? "正在办理手续…"
            : authMode === "login"
              ? "登录，开始精神离职 →"
              : "注册并进入食堂 →"
        }}
      </button>
      <text class="modal-footnote"
        >{{ loading ? "正在检查登录状态…" : "新账号赠送300精神值 · 仅模拟交易"
        }}<br />请记好账号密码，暂不提供自助找回。</text
      >
    </view>
  </view>
  <view v-else class="app-shell" :class="{ 'has-player': playerState.track }">
    <view class="topbar"
      ><view class="brand" @click="tab = 'menu'"
        ><view class="brand-icon">离</view
        ><view
          ><text class="brand-name">精神离职</text
          ><text class="brand-en">MIND OFFLINE</text></view
        ></view
      ><view class="top-right"
        ><text class="account-name">@{{ me.username }}</text
        ><button class="logout-button" :disabled="busy" @click="signOut">
          退出</button
        ><text class="open-dot"></text><text>人类补给中</text
        ><text class="edition">VOL. 001 / 不想上班</text></view
      ></view
    >
    <view class="layout">
      <view class="sidebar">
        <view class="side-title">今日营业，精神随缘。</view>
        <view
          class="nav-item"
          :class="{ active: tab === 'menu' }"
          @click="tab = 'menu'"
          ><text>↗</text><text>补给菜单</text
          ><text class="nav-index">01</text></view
        >
        <view
          class="nav-item"
          :class="{ active: tab === 'orders' }"
          @click="
            tab = 'orders';
            refresh();
          "
          ><text>≡</text><text>我的离职单</text
          ><text class="nav-index">{{ active || "02" }}</text></view
        >
        <view
          class="nav-item"
          :class="{ active: tab === 'music' }"
          @click="tab = 'music'"
          ><text>♫</text><text>音乐实验室</text
          ><text class="nav-index">03</text></view
        >
        <view
          class="nav-item"
          :class="{ active: tab === 'about' }"
          @click="tab = 'about'"
          ><text>✳</text><text>食堂生存指南</text
          ><text class="nav-index">04</text></view
        >
        <view class="wallet"
          ><text class="eyebrow">精神补偿金 / WALLET</text
          ><view class="wallet-value">{{ me.balance }}<text>精神值</text></view
          ><text class="wallet-note">钱可以买快乐。这里的钱是假的。</text
          ><button
            class="claim"
            :disabled="me.claimed_today || busy || loading"
            @click="claim"
          >
            {{ me.claimed_today ? "✓ 今日补给已领取" : "+ 领取今日补给 100" }}
          </button></view
        >
        <view class="side-note"
          ><text class="note-star">✳</text
          ><text>本店不解决问题，<br />只提供餐具。</text
          ><text class="tiny">纯属虚构 · 请放心摸鱼</text></view
        >
      </view>
      <view class="main">
        <view v-if="error" class="error-banner" @click="refresh"
          >{{ error }} · 点此重试</view
        >
        <template v-if="tab === 'menu'">
          <view class="hero"
            ><view class="hero-copy"
              ><view class="hero-badge">打工人情绪补给站 <text>↗</text></view
              ><view class="hero-title"
                >身体在工位，<br /><text>精神已离职。</text></view
              ><view class="hero-description"
                >今日份内耗，到饭点为止。<br />选一份精神食粮，给自己放个小假。</view
              ><view class="hero-foot"
                ><text class="hero-pill">✦ 仅收精神值</text
                ><text>不卷了，开饭吧 →</text></view
              ></view
            ><view class="hero-art"
              ><view class="orbit orbit-one"></view
              ><view class="orbit orbit-two"></view
              ><view class="plate"
                ><view class="plate-inner"
                  ><text class="food-hero">🍜</text></view
                ></view
              ><view class="sticker">拒绝内耗<br /><text>多吃两口</text></view
              ><text class="sparkle">✳</text
              ><text class="art-caption"
                >100% 情绪友好 / 0% 真实交易</text
              ></view
            ></view
          >
          <view class="ticker"
            ><text>✳ 不接收工作消息</text
            ><text>✳ 不提供老板画的饼以外的饼</text
            ><text>✳ 吃饱再考虑人生</text></view
          >
          <view class="section-heading"
            ><view
              ><text class="eyebrow">THE OFFLINE MENU</text
              ><view class="section-title">今天，靠什么续命？</view></view
            ><text class="section-number"
              >{{ menu.length }} 款精神补给</text
            ></view
          >
          <scroll-view scroll-x class="categories"
            ><view class="category-row"
              ><button
                v-for="c in categories"
                :key="c"
                class="category"
                :class="{ selected: category === c }"
                @click="category = c"
              >
                {{ c }}
              </button></view
            ></scroll-view
          >
          <view v-if="loading" class="empty">食堂正在掀锅盖…</view>
          <view v-else-if="!filtered.length" class="empty"
            >这个窗口还在备菜，换一个看看。</view
          >
          <view class="dish-grid"
            ><view
              v-for="(d, i) in filtered"
              :key="d.id"
              class="dish-card"
              :class="{ sold: !d.available }"
              ><view
                class="dish-art"
                :class="'tone-' + (d.id % 4)"
                @click="choose(d)"
                ><text class="dish-tag">{{
                  !d.available
                    ? "暂时售罄"
                    : d.id === 1
                      ? "精神主推"
                      : d.category
                }}</text
                ><text class="dish-emoji">{{ d.emoji }}</text
                ><text class="dish-art-word">{{
                  ["OFFLINE", "TAKE A BREAK", "NO MORE KPI", "STAY SOFT"][
                    d.id % 4
                  ]
                }}</text></view
              ><view class="dish-info"
                ><view class="dish-name">{{ d.name }}</view
                ><view class="dish-description">{{ d.description }}</view
                ><view class="dish-bottom"
                  ><view class="price">{{ d.price }}<text>精神值</text></view
                  ><button
                    class="add-button"
                    :disabled="!d.available"
                    :aria-label="'选择' + d.name"
                    @click="choose(d)"
                  >
                    +
                  </button></view
                ></view
              ></view
            ></view
          >
        </template>
        <template v-if="tab === 'orders'"
          ><view class="page-heading"
            ><text class="eyebrow">MY OFFLINE RECORDS</text
            ><view class="section-title">我的离职单</view
            ><text class="muted"
              >老板可以不回消息，食堂会认真处理你的订单。</text
            ></view
          ><view v-if="!orders.length" class="empty"
            ><text class="empty-emoji">🛋️</text><view>还没办理过精神离职</view
            ><text class="muted">去菜单里挑一份快乐吧。</text
            ><button class="primary" @click="tab = 'menu'">
              去补给 →
            </button></view
          ><view v-for="o in orders" :key="o.id" class="order-card"
            ><view class="order-top"
              ><text class="order-no">{{ number(o) }}</text
              ><text class="status-pill" :class="o.status">{{
                labels[o.status]
              }}</text></view
            ><view class="order-items"
              ><view v-for="(i, k) in o.items" :key="k"
                ><text>{{ i.emoji }} {{ i.name }}</text
                ><text class="muted"
                  >{{ i.mood }} × {{ i.quantity }}</text
                ></view
              ></view
            ><view v-if="o.status !== 'cancelled'" class="progress"
              ><view
                v-for="(s, i) in steps"
                :key="s"
                :class="{ done: steps.indexOf(o.status) >= i }"
                ><view class="progress-line"></view
                ><text>{{
                  ["已提交", "制作中", "可取餐", "已完成"][i]
                }}</text></view
              ></view
            ><view class="order-bottom"
              ><text class="muted"
                >{{ date(o.created_at) }} · {{ o.total }} 精神值</text
              ><button class="outline" @click="receipt = o">
                查看离职小票 ↗
              </button></view
            ></view
          ></template
        >
        <MusicPanel
          v-if="tab === 'music'"
          :key="me.id"
          :account-id="me.id"
          :foreground="musicForeground"
          @auth-expired="forgetSession"
        />
        <template v-if="tab === 'about'"
          ><view class="page-heading"
            ><text class="eyebrow">EMPLOYEE SURVIVAL GUIDE</text
            ><view class="section-title">欢迎合法精神离职。</view></view
          ><view class="guide-card"
            ><text class="guide-big">✳</text
            ><view class="section-title">人可以不清醒，<br />饭不能不吃。</view
            ><view class="guide-text"
              >这是一个不卖真饭、不收真钱的虚拟食堂。<br />我们认真模拟点单，也认真允许你摸一会儿鱼。</view
            ><view class="guide-rule"
              ><text>01 / 精神值怎么来？</text
              ><text
                >新账号注册获得300精神值，每天可再领取100。仅用于本站体验，没有现金价值。</text
              ></view
            ><view class="guide-rule"
              ><text>02 / 下单后会发生什么？</text
              ><text
                >店长在后台接单、制作和出餐，你可以在“我的离职单”查看进度。店长没上线时，订单会等待处理。</text
              ></view
            ><view class="guide-rule"
              ><text>03 / 我的订单保存在哪里？</text
              ><text
                >订单和精神值归属于你的账号。换设备后登录同一账号即可查看；退出后本页会清空个人信息，其他账号看不到你的订单。</text
              ></view
            ><view class="guide-rule"
              ><text>04 / 要填手机号和地址吗？</text
              ><text
                >不用。没有真实配送，也请不要在备注里填写个人敏感信息。</text
              ></view
            ></view
          ></template
        >
        <view class="footer"
          ><text>© MIND OFFLINE / 精神离职</text
          ><text>生活没有标准答案，菜单有。</text></view
        >
      </view>
    </view>
    <MiniPlayer @open="tab = 'music'" />
    <view v-if="count && tab === 'menu'" class="cart-bar"
      ><view class="cart-icon"
        >袋<text>{{ count }}</text></view
      ><view class="cart-bar-price"
        >{{ total }}<text>精神值 · {{ count }} 份补给</text></view
      ><button @click="cartOpen = true">办理精神离职 →</button></view
    >
    <view v-if="selected" class="overlay" @click="selected = null"
      ><view class="modal dish-modal" @click.stop
        ><button class="close" @click="selected = null">×</button
        ><view class="modal-emoji">{{ selected.emoji }}</view
        ><text class="eyebrow">YOUR EMOTIONAL SUPPORT FOOD</text
        ><view class="section-title">{{ selected.name }}</view
        ><text class="muted">{{ selected.description }}</text
        ><view class="field-label">选择今日精神状态</view
        ><view class="mood-options"
          ><button
            v-for="m in moods"
            :key="m"
            :class="{ picked: mood === m }"
            @click="mood = m"
          >
            {{ m }}
          </button></view
        ><view class="quantity-row"
          ><text>快乐的份数</text
          ><view class="stepper"
            ><button :disabled="quantity <= 1" @click="quantity--">−</button
            ><text>{{ quantity }}</text
            ><button :disabled="quantity >= 10" @click="quantity++">
              +
            </button></view
          ></view
        ><button class="primary wide" @click="add">
          加入补给袋 · {{ selected.price * quantity }} 精神值</button
        ><text class="modal-footnote">模拟点单，不会产生真实消费。</text></view
      ></view
    >
    <view v-if="cartOpen" class="overlay" @click="!busy && (cartOpen = false)"
      ><view class="modal cart-modal" @click.stop
        ><button class="close" :disabled="busy" @click="cartOpen = false">
          ×</button
        ><text class="eyebrow">YOUR OFFLINE BAG</text
        ><view class="section-title">本次离职补给</view
        ><scroll-view scroll-y class="cart-list"
          ><view
            v-for="(i, k) in cart"
            :key="i.dish_id + i.mood"
            class="cart-line"
            ><text class="cart-food">{{ i.emoji }}</text
            ><view class="cart-line-name"
              ><text>{{ i.name }}</text
              ><text class="muted"
                >{{ i.mood }} · {{ i.price }} 精神值</text
              ></view
            ><view class="stepper"
              ><button :disabled="busy" @click="change(k, -1)">−</button
              ><text>{{ i.quantity }}</text
              ><button :disabled="busy" @click="change(k, 1)">+</button></view
            ></view
          ><view v-if="!cart.length" class="empty"
            >补给袋空了，先去选点快乐。</view
          ></scroll-view
        ><view class="field-label"
          >给厨师的一句话 <text class="muted">（选填）</text></view
        ><textarea
          v-model="note"
          class="note-input"
          maxlength="100"
          placeholder="例如：不要香菜，不要老板画的饼。"
          :disabled="busy"
        /><view class="checkout-total"
          ><text
            >合计 <text class="price">{{ total }}</text> 精神值</text
          ><text class="muted">余额 {{ me.balance }}</text></view
        ><button
          class="primary wide"
          :disabled="busy || !cart.length || total > me.balance"
          @click="checkout"
        >
          {{
            busy
              ? "离职手续办理中…"
              : total > me.balance
                ? "精神值不足，请领取补给"
                : "确认模拟下单 →"
          }}</button
        ><text class="modal-footnote"
          >最终价格以服务器结算为准 · 无真实支付</text
        ></view
      ></view
    >
    <view v-if="receipt" class="overlay" @click="receipt = null"
      ><view class="modal receipt-modal" @click.stop
        ><button class="close" @click="receipt = null">×</button
        ><view class="receipt-brand">精神离职</view
        ><text class="receipt-en">CERTIFICATE OF MENTAL RESIGNATION</text
        ><view class="receipt-stamp">批准<br />离线</view
        ><view class="receipt-divider"></view
        ><view class="receipt-row"
          ><text>补给编号</text><text>{{ number(receipt) }}</text></view
        ><view class="receipt-row"
          ><text>办理时间</text
          ><text>{{ date(receipt.created_at) }}</text></view
        ><view class="receipt-row"
          ><text>精神状态</text><text>{{ receipt.mood }}</text></view
        ><view class="receipt-divider"></view
        ><scroll-view scroll-y class="receipt-lines"
          ><view v-for="(i, k) in receipt.items" :key="k" class="receipt-row"
            ><text>{{ i.name }} × {{ i.quantity }}</text
            ><text>{{ i.price! * i.quantity }}</text></view
          ></scroll-view
        ><view class="receipt-row receipt-total"
          ><text>精神支出</text><text>{{ receipt.total }} 点</text></view
        ><text v-if="receipt.note" class="receipt-note"
          >备注：{{ receipt.note }}</text
        ><view class="receipt-quote">“{{ receipt.quote }}”</view
        ><view class="barcode">┃│┃┃│┃││┃┃┃│┃│┃┃││┃┃│┃│┃┃│┃</view
        ><text class="modal-footnote"
          >虚拟体验凭证 · 没有真饭，但快乐是真的</text
        ><button class="primary wide" @click="saveReceipt">
          ↓ 保存我的离职小票
        </button></view
      ></view
    >
    <canvas
      canvas-id="receiptCanvas"
      id="receiptCanvas"
      style="
        width: 360px;
        height: 600px;
        position: fixed;
        left: -2000px;
        top: 0;
      "
    />
  </view>
</template>

<style>
.auth-page {
  min-height: 100vh;
  display: flex;
  max-width: 1440px;
  margin: auto;
}
.auth-story {
  width: 52%;
  background: #e7e0f2;
  padding: 70px 55px;
  display: flex;
  flex-direction: column;
  justify-content: center;
  position: relative;
  overflow: hidden;
}
.auth-wordmark {
  font-size: 14px;
  letter-spacing: 2px;
  position: absolute;
  top: 45px;
}
.auth-title {
  font-size: 42px;
  font-weight: 900;
  line-height: 1.5;
  z-index: 1;
  letter-spacing: -1px;
}
.auth-title text {
  color: #6e55a4;
}
.auth-star {
  font-size: 230px;
  line-height: 1.2;
  color: #c7d987;
  align-self: flex-end;
  transform: rotate(15deg);
}
.auth-subtitle {
  font-size: 13px;
  color: #8b7e9a;
}
.auth-card {
  width: 390px;
  max-width: calc(100% - 40px);
  margin: auto;
  padding: 40px 0;
}
.auth-tabs {
  display: flex;
  margin: 22px 0 8px;
  border-bottom: 1px solid #d9d8cd;
}
.auth-tabs button {
  flex: 1;
  padding: 12px;
  font-size: 13px;
  color: #8a8b7b;
  border-bottom: 2px solid transparent;
}
.auth-tabs .picked {
  color: #6550a0;
  border-color: #6550a0;
}
.auth-input {
  width: 100%;
  height: 45px;
  border: 1px solid #d7d9cb;
  border-radius: 5px;
  background: #faf9f2;
  padding: 0 12px;
  font-size: 13px;
}
.auth-card .primary {
  margin-top: 25px;
}
.auth-error {
  display: block;
  font-size: 12px;
  color: #a15136;
  line-height: 1.8;
  margin-top: 16px;
}
.legacy-option {
  font-size: 12px;
  line-height: 1.8;
  margin-top: 20px;
  color: #66518d;
  cursor: pointer;
}
.legacy-option text {
  display: block;
}
.logout-button {
  font-size: 11px;
  border: 1px solid #cbc7d5;
  padding: 5px 9px;
  border-radius: 4px;
}
.account-name {
  font-size: 12px;
  color: #6e55a4;
  max-width: 150px;
  overflow: hidden;
  text-overflow: ellipsis;
}
.auth-card .field-label {
  margin-top: 18px;
}
@media (max-width: 760px) {
  .auth-page {
    display: block;
  }
  .auth-story {
    width: 100%;
    padding: 70px 25px 28px;
    min-height: 240px;
  }
  .auth-wordmark {
    top: 28px;
    font-size: 11px;
  }
  .auth-title {
    font-size: 28px;
  }
  .auth-star {
    font-size: 150px;
    position: absolute;
    right: 0;
    bottom: 10px;
    opacity: 0.5;
  }
  .auth-subtitle {
    font-size: 10px;
    margin-top: 18px;
    z-index: 1;
  }
  .auth-card {
    width: 380px;
    padding: 28px 0;
  }
  .account-name {
    font-size: 10px;
    max-width: 95px;
  }
  .top-right .open-dot,
  .top-right > .edition {
    display: none;
  }
  .logout-button {
    font-size: 10px;
  }
}

.app-shell {
  max-width: 1440px;
  margin: 0 auto;
  padding: 0 48px;
}
.topbar {
  height: 100px;
  border-bottom: 1px solid #d9d8cd;
  display: flex;
  align-items: center;
  justify-content: space-between;
}
.brand {
  display: flex;
  gap: 12px;
  align-items: center;
  cursor: pointer;
}
.brand-icon {
  background: #d9f078;
  width: 44px;
  height: 44px;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 25px;
  border: 1.5px solid #23271f;
  box-shadow: 3px 3px 0 #23271f;
  transform: rotate(-5deg);
  font-weight: 900;
}
.brand-name {
  display: block;
  font-size: 23px;
  font-weight: 900;
  letter-spacing: 2px;
}
.brand-en {
  display: block;
  font-size: 9px;
  letter-spacing: 3px;
  margin-top: 4px;
}
.top-right {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 12px;
}
.open-dot {
  width: 6px;
  height: 6px;
  background: #658952;
  border-radius: 50%;
}
.edition {
  margin-left: 25px;
  color: #838578;
  font-size: 10px;
  letter-spacing: 1px;
}
.layout {
  display: flex;
  gap: 40px;
  padding-top: 32px;
}
.sidebar {
  width: 205px;
  flex-shrink: 0;
}
.side-title {
  font-size: 11px;
  color: #858776;
  margin: 8px 0 25px;
  letter-spacing: 1px;
}
.nav-item {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 16px 13px;
  margin: 8px 0;
  cursor: pointer;
  font-weight: 600;
  font-size: 13px;
  border-radius: 6px;
}
.nav-item.active {
  background: #272c25;
  color: #f8f7f0;
}
.nav-item > text:first-child {
  font-size: 21px;
  font-weight: 400;
}
.nav-index {
  margin-left: auto;
  font-size: 10px;
  opacity: 0.6;
}
.wallet {
  margin-top: 40px;
  padding: 21px 17px;
  background: #eae5f4;
  border: 1px solid #dcd4eb;
  border-radius: 8px;
}
.eyebrow {
  font-size: 9px;
  letter-spacing: 1.6px;
  font-weight: 600;
  color: #74796b;
}
.wallet-value {
  font-size: 42px;
  letter-spacing: -2px;
  margin: 12px 0;
  color: #6450a3;
  font-weight: 800;
}
.wallet-value text {
  font-size: 11px;
  font-weight: 400;
  letter-spacing: 0;
  margin-left: 9px;
}
.wallet-note {
  font-size: 10px;
  line-height: 1.8;
  display: block;
  color: #777083;
}
.claim {
  font-size: 11px;
  border: 1px solid #aaa0c7;
  border-radius: 4px;
  padding: 10px 5px;
  margin-top: 16px;
  width: 100%;
  color: #554785;
  cursor: pointer;
}
.side-note {
  margin-top: 45px;
  display: flex;
  flex-direction: column;
  gap: 18px;
  font-size: 13px;
  line-height: 1.9;
  color: #737766;
  padding-left: 16px;
}
.note-star {
  font-size: 40px;
  color: #8b9674;
}
.tiny {
  font-size: 9px;
  color: #969988;
}
.main {
  flex: 1;
  min-width: 0;
}
.hero {
  display: flex;
  position: relative;
  overflow: hidden;
  background: #e7e0f2;
  border-radius: 10px;
  min-height: 310px;
  padding: 30px 34px;
}
.hero-copy {
  z-index: 1;
  flex: 1;
}
.hero-badge {
  font-size: 11px;
  display: inline-flex;
  gap: 14px;
  align-items: center;
  border: 1px solid #aba0c8;
  border-radius: 20px;
  padding: 6px 12px;
  color: #635781;
}
.hero-title {
  font-size: 39px;
  line-height: 1.3;
  letter-spacing: -1.6px;
  font-weight: 900;
  margin: 22px 0 15px;
}
.hero-title > text {
  color: #6e55a4;
}
.hero-description {
  font-size: 12px;
  color: #76717f;
  line-height: 1.9;
}
.hero-foot {
  display: flex;
  align-items: center;
  gap: 18px;
  margin-top: 25px;
  font-size: 10px;
  color: #5a4f6a;
}
.hero-pill {
  background: #d7ed7e;
  border: 1px solid #bccc6c;
  padding: 5px 8px;
  border-radius: 4px;
  color: #434e2c;
}
.hero-art {
  width: 290px;
  position: relative;
  display: flex;
  justify-content: center;
  align-items: center;
}
.plate {
  width: 205px;
  height: 205px;
  border-radius: 50%;
  background: #f9f8eb;
  box-shadow:
    8px 13px 0 #beb3d680,
    0 0 0 1px #fefcf4;
  transform: rotate(-12deg);
  display: flex;
  align-items: center;
  justify-content: center;
}
.plate-inner {
  border: 2px solid #d9dcc7;
  border-radius: 50%;
  width: 166px;
  height: 166px;
  display: flex;
  align-items: center;
  justify-content: center;
}
.food-hero {
  font-size: 112px;
  filter: drop-shadow(3px 9px 5px #a2917c40);
}
.orbit {
  position: absolute;
  border: 1px dashed #c5b8d7;
  border-radius: 50%;
  width: 275px;
  height: 275px;
}
.orbit-two {
  width: 305px;
  height: 305px;
}
.sticker {
  position: absolute;
  right: 0;
  bottom: 45px;
  background: #d9ef87;
  padding: 12px 16px;
  transform: rotate(9deg);
  border: 1px solid #717a42;
  box-shadow: 3px 3px 0 #7b73612a;
  font-weight: 800;
  line-height: 1.5;
  font-size: 17px;
}
.sticker text {
  font-size: 11px;
  font-weight: 500;
}
.sparkle {
  position: absolute;
  top: 8px;
  left: 6px;
  font-size: 50px;
  color: #6851a2;
}
.art-caption {
  position: absolute;
  bottom: 0;
  font-size: 8px;
  letter-spacing: 1.4px;
  color: #8a7d9b;
}
.ticker {
  display: flex;
  justify-content: space-between;
  gap: 12px;
  border-bottom: 1px solid #d9d8cd;
  padding: 17px 4px;
  font-size: 10px;
  color: #7d816e;
}
.section-heading {
  display: flex;
  justify-content: space-between;
  align-items: end;
  margin: 30px 0 22px;
}
.section-title {
  font-size: 24px;
  font-weight: 800;
  letter-spacing: -0.5px;
  margin: 8px 0 10px;
  line-height: 1.4;
}
.section-number {
  font-size: 10px;
  color: #909281;
  padding-bottom: 12px;
}
.category-row {
  display: flex;
  gap: 8px;
  padding-bottom: 20px;
  white-space: nowrap;
}
.category {
  border: 1px solid #d9dacb;
  padding: 9px 15px;
  border-radius: 5px;
  font-size: 11px;
  cursor: pointer;
  flex-shrink: 0;
}
.category.selected {
  background: #d9ed8a;
  border-color: #b1c661;
  color: #424b2b;
  font-weight: 600;
}
.dish-grid {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 20px;
}
.dish-card {
  background: #faf9f2;
  border: 1px solid #ddded0;
  border-radius: 9px;
  overflow: hidden;
  transition: transform 0.2s;
}
.dish-card:hover {
  transform: translateY(-3px);
}
.dish-art {
  height: 163px;
  position: relative;
  display: flex;
  align-items: center;
  justify-content: center;
  cursor: pointer;
  overflow: hidden;
}
.tone-0 {
  background: #e5dfec;
}
.tone-1 {
  background: #e9e7d7;
}
.tone-2 {
  background: #e5ebd9;
}
.tone-3 {
  background: #efdccf;
}
.dish-tag {
  position: absolute;
  top: 12px;
  left: 12px;
  font-size: 9px;
  border: 1px solid #c3c7b5;
  padding: 3px 7px;
  border-radius: 3px;
  color: #657157;
  background: #ffffff44;
}
.dish-emoji {
  font-size: 81px;
  transform: rotate(-10deg);
  filter: drop-shadow(5px 9px 3px #4b433224);
}
.dish-art-word {
  position: absolute;
  bottom: 10px;
  right: 10px;
  font-size: 8px;
  letter-spacing: 1.5px;
  color: #7a7e6a88;
}
.dish-info {
  padding: 17px;
}
.dish-name {
  font-weight: 750;
  font-size: 15px;
}
.dish-description {
  font-size: 10px;
  color: #8a8b7f;
  line-height: 1.7;
  min-height: 35px;
  margin: 9px 0 13px;
}
.dish-bottom {
  display: flex;
  align-items: center;
  justify-content: space-between;
}
.price {
  font-size: 23px;
  font-weight: 800;
  color: #353e2b;
}
.price text {
  font-size: 9px;
  font-weight: 400;
  color: #8a8b7f;
  margin-left: 5px;
}
.add-button {
  background: #d9ed8a;
  width: 30px;
  height: 30px;
  border: 1px solid #b8c979;
  border-radius: 50%;
  line-height: 27px;
  font-size: 22px;
  text-align: center;
  cursor: pointer;
}
.sold {
  opacity: 0.6;
}
.footer {
  border-top: 1px solid #d9d8cd;
  display: flex;
  justify-content: space-between;
  font-size: 9px;
  color: #9b9e8e;
  margin-top: 40px;
  padding: 22px 0 110px;
  letter-spacing: 0.5px;
}
.cart-bar {
  position: fixed;
  bottom: 24px;
  left: 50%;
  transform: translateX(-50%);
  width: min(640px, calc(100% - 36px));
  display: flex;
  align-items: center;
  background: #292e27;
  color: #f5f5e9;
  border-radius: 12px;
  padding: 12px 14px;
  z-index: 10;
  box-shadow: 0 8px 30px #21251f25;
}
.cart-icon {
  width: 42px;
  height: 42px;
  background: #454c3b;
  border-radius: 8px;
  display: flex;
  align-items: center;
  justify-content: center;
  position: relative;
  color: #d9ed8a;
}
.cart-icon text {
  position: absolute;
  right: -4px;
  top: -5px;
  background: #d9ed8a;
  color: #303726;
  font-size: 10px;
  padding: 2px 5px;
  border-radius: 10px;
}
.cart-bar-price {
  font-size: 24px;
  font-weight: 700;
  margin-left: 16px;
}
.cart-bar-price text {
  display: block;
  font-size: 9px;
  color: #b5bdab;
  font-weight: 400;
  margin-top: 2px;
}
.cart-bar > button {
  margin-left: auto;
  background: #d9ed8a;
  color: #303726;
  font-size: 12px;
  font-weight: 700;
  padding: 15px 20px;
  border-radius: 6px;
  cursor: pointer;
}
.overlay {
  position: fixed;
  inset: 0;
  background: #25281d88;
  backdrop-filter: blur(5px);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 30;
  padding: 20px;
}
.modal {
  background: #faf9f2;
  border-radius: 14px;
  padding: 30px;
  width: 440px;
  max-width: 100%;
  max-height: 90vh;
  overflow-y: auto;
  position: relative;
  box-shadow: 0 20px 70px #0003;
}
.close {
  position: absolute;
  top: 12px;
  right: 16px;
  font-size: 28px;
  color: #8a8d7d;
  cursor: pointer;
  z-index: 1;
}
.modal-emoji {
  font-size: 75px;
  text-align: center;
  background: #eae6f0;
  border-radius: 8px;
  margin: 0 0 22px;
  padding: 15px;
}
.muted {
  font-size: 11px;
  line-height: 1.8;
  color: #858875;
}
.field-label {
  font-size: 12px;
  font-weight: 600;
  margin: 22px 0 12px;
}
.mood-options {
  display: flex;
  gap: 8px;
}
.mood-options button {
  font-size: 11px;
  padding: 10px 8px;
  border: 1px solid #dadbcd;
  border-radius: 5px;
  flex: 1;
  cursor: pointer;
}
.mood-options .picked {
  background: #e8e0f2;
  color: #695399;
  border-color: #b7a5d3;
}
.quantity-row {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin: 24px 0;
  font-size: 12px;
}
.stepper {
  display: flex;
  align-items: center;
  gap: 12px;
  font-size: 13px;
}
.stepper button {
  border: 1px solid #d8dcca;
  background: #f1f1e6;
  border-radius: 4px;
  width: 27px;
  height: 27px;
  line-height: 25px;
  text-align: center;
  cursor: pointer;
}
.primary {
  background: #6550a0;
  color: #fff;
  padding: 14px 20px;
  border-radius: 6px;
  font-size: 12px;
  font-weight: 600;
  cursor: pointer;
}
.wide {
  width: 100%;
}
.modal-footnote {
  display: block;
  font-size: 9px;
  color: #979989;
  text-align: center;
  margin: 14px 0 0;
  line-height: 1.8;
}
.cart-list {
  max-height: 280px;
}
.cart-line {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 15px 0;
  border-bottom: 1px solid #e7e7da;
}
.cart-food {
  font-size: 29px;
}
.cart-line-name {
  flex: 1;
  font-size: 12px;
  display: flex;
  flex-direction: column;
  gap: 4px;
}
.note-input {
  background: #f0efe5;
  border: 1px solid #dcdecb;
  border-radius: 5px;
  padding: 12px;
  font-size: 12px;
  width: 100%;
  height: 70px;
}
.checkout-total {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 20px 0;
  font-size: 12px;
}
.checkout-total .price {
  margin: 0 5px;
}
.page-heading {
  padding: 15px 0 25px;
}
.order-card {
  background: #faf9f2;
  border: 1px solid #d9dccd;
  border-radius: 8px;
  padding: 22px;
  margin: 0 0 18px;
}
.order-top {
  display: flex;
  justify-content: space-between;
  gap: 10px;
  align-items: center;
}
.order-no {
  font-weight: 800;
  font-size: 15px;
}
.status-pill {
  font-size: 10px;
  padding: 6px 9px;
  background: #eae4f4;
  color: #69559c;
  border-radius: 20px;
}
.status-pill.ready {
  background: #e2edc7;
  color: #586c34;
}
.status-pill.cancelled {
  background: #eae8df;
  color: #8d8b81;
}
.order-items {
  margin: 20px 0;
}
.order-items > view {
  display: flex;
  justify-content: space-between;
  gap: 10px;
  margin: 12px 0;
  font-size: 13px;
}
.progress {
  display: flex;
  margin: 24px 0;
}
.progress > view {
  flex: 1;
  color: #abada2;
  font-size: 10px;
}
.progress-line {
  height: 3px;
  background: #e2e5d9;
  margin: 0 4px 10px 0;
  border-radius: 3px;
}
.progress .done {
  color: #66539c;
}
.progress .done .progress-line {
  background: #ab98d0;
}
.order-bottom {
  display: flex;
  justify-content: space-between;
  align-items: center;
  border-top: 1px solid #e5e6d9;
  padding-top: 15px;
  gap: 8px;
}
.outline {
  border: 1px solid #bbb6cc;
  font-size: 10px;
  padding: 8px 10px;
  border-radius: 4px;
  cursor: pointer;
  color: #66539c;
}
.empty {
  padding: 50px 20px;
  text-align: center;
  line-height: 2.5;
  color: #767d68;
  font-size: 14px;
}
.empty-emoji {
  font-size: 65px;
  display: block;
}
.empty .primary {
  display: block;
  margin: 20px auto;
  max-width: 180px;
}
.guide-card {
  background: #e9e3f0;
  padding: 35px;
  border-radius: 10px;
}
.guide-big {
  color: #69529f;
  font-size: 60px;
}
.guide-text {
  line-height: 2;
  color: #837a8f;
  font-size: 13px;
  margin: 20px 0 30px;
}
.guide-rule {
  padding: 20px 0;
  border-top: 1px solid #cfc5db;
  display: flex;
  flex-direction: column;
  gap: 10px;
  line-height: 1.9;
  font-size: 12px;
}
.guide-rule > text:first-child {
  font-weight: 700;
  color: #5d4b7e;
}
.guide-rule > text:last-child {
  color: #817788;
}
.receipt-modal {
  border-radius: 3px;
  padding: 36px 30px;
  border-top: 7px solid #6851a3;
  width: 380px;
}
.receipt-brand {
  font-size: 28px;
  font-weight: 900;
  letter-spacing: 3px;
}
.receipt-en {
  font-size: 7px;
  letter-spacing: 1px;
  color: #96968b;
  display: block;
  margin: 8px 0 26px;
}
.receipt-stamp {
  position: absolute;
  top: 72px;
  right: 30px;
  width: 64px;
  height: 64px;
  border: 2px solid #806bb499;
  border-radius: 50%;
  color: #806bb4;
  text-align: center;
  font-size: 15px;
  line-height: 1.5;
  padding-top: 7px;
  transform: rotate(-15deg);
  font-weight: 700;
}
.receipt-divider {
  border-top: 1px dashed #c8ccbc;
  margin: 18px 0;
}
.receipt-row {
  display: flex;
  justify-content: space-between;
  gap: 10px;
  font-size: 11px;
  line-height: 2.5;
  color: #777f6a;
}
.receipt-row > text:first-child {
  max-width: 75%;
}
.receipt-total {
  font-size: 17px;
  font-weight: 700;
  color: #353e2c;
  border-top: 1px dashed #c8ccbc;
  margin-top: 15px;
  padding-top: 12px;
}
.receipt-lines {
  max-height: 190px;
}
.receipt-quote {
  padding: 20px 8px;
  font-size: 14px;
  color: #6a559e;
  text-align: center;
  line-height: 1.9;
}
.barcode {
  text-align: center;
  font-size: 30px;
  letter-spacing: 1px;
  overflow: hidden;
  white-space: nowrap;
}
.receipt-modal .primary {
  margin-top: 20px;
}
.receipt-note {
  font-size: 10px;
  line-height: 1.7;
  color: #8a8e7b;
  display: block;
}
.error-banner {
  background: #f4ddd1;
  color: #985736;
  padding: 12px;
  margin-bottom: 14px;
  border-radius: 5px;
  font-size: 12px;
  cursor: pointer;
}
@media (min-width: 1400px) {
  .hero {
    min-height: 340px;
  }
  .hero-title {
    font-size: 46px;
  }
  .hero-art {
    width: 340px;
  }
  .dish-art {
    height: 185px;
  }
}
@media (max-width: 1050px) {
  .app-shell {
    padding: 0 25px;
  }
  .layout {
    gap: 25px;
  }
  .sidebar {
    width: 180px;
  }
  .hero {
    padding: 25px;
  }
  .hero-title {
    font-size: 30px;
  }
  .hero-art {
    width: 200px;
  }
  .plate {
    width: 160px;
    height: 160px;
  }
  .plate-inner {
    width: 135px;
    height: 135px;
  }
  .food-hero {
    font-size: 88px;
  }
  .orbit {
    width: 205px;
    height: 205px;
  }
  .orbit-two {
    width: 235px;
    height: 235px;
  }
  .sticker {
    font-size: 14px;
    right: -8px;
  }
  .dish-grid {
    gap: 12px;
  }
  .dish-info {
    padding: 13px;
  }
  .dish-name {
    font-size: 13px;
  }
  .ticker {
    font-size: 9px;
  }
  .dish-art {
    height: 140px;
  }
}
@media (max-width: 760px) {
  .app-shell {
    padding: 0 18px;
  }
  .topbar {
    height: 82px;
    padding-top: env(safe-area-inset-top);
  }
  .brand-name {
    font-size: 19px;
  }
  .brand-icon {
    width: 36px;
    height: 36px;
    font-size: 21px;
  }
  .brand-en {
    font-size: 7px;
    letter-spacing: 2px;
  }
  .top-right {
    font-size: 10px;
  }
  .edition {
    display: none;
  }
  .layout {
    display: block;
    padding-top: 14px;
  }
  .sidebar {
    width: 100%;
    display: flex;
    flex-wrap: wrap;
    gap: 5px;
  }
  .side-title,
  .side-note {
    display: none;
  }
  .nav-item {
    flex: 1;
    justify-content: center;
    gap: 6px;
    font-size: 11px;
    padding: 10px 5px;
    margin: 0;
    white-space: nowrap;
  }
  .nav-item > text:first-child {
    font-size: 16px;
  }
  .nav-index {
    display: none;
  }
  .wallet {
    width: 100%;
    margin: 12px 0 15px;
    padding: 11px 13px;
    display: flex;
    align-items: center;
    gap: 10px;
    border-radius: 6px;
  }
  .wallet .eyebrow,
  .wallet-note {
    display: none;
  }
  .wallet-value {
    font-size: 24px;
    margin: 0;
    letter-spacing: -0.5px;
  }
  .wallet-value text {
    font-size: 10px;
  }
  .claim {
    margin: 0 0 0 auto;
    width: auto;
    padding: 7px 10px;
    font-size: 10px;
  }
  .hero {
    min-height: 245px;
    padding: 24px 20px;
    border-radius: 8px;
  }
  .hero-title {
    font-size: 29px;
    margin: 16px 0 12px;
  }
  .hero-badge {
    font-size: 9px;
    padding: 5px 8px;
    gap: 7px;
  }
  .hero-description {
    font-size: 10px;
    line-height: 1.8;
  }
  .hero-foot {
    font-size: 9px;
    gap: 8px;
    margin-top: 18px;
  }
  .hero-art {
    position: absolute;
    width: 155px;
    right: -16px;
    top: 60px;
    height: 165px;
    opacity: 0.95;
  }
  .plate {
    width: 130px;
    height: 130px;
  }
  .plate-inner {
    width: 107px;
    height: 107px;
  }
  .food-hero {
    font-size: 69px;
  }
  .orbit {
    width: 160px;
    height: 160px;
  }
  .orbit-two {
    width: 185px;
    height: 185px;
  }
  .sticker {
    bottom: -13px;
    right: 14px;
    font-size: 12px;
    padding: 7px 10px;
  }
  .sticker text {
    font-size: 9px;
  }
  .sparkle {
    font-size: 28px;
    top: -22px;
    left: 75px;
  }
  .art-caption {
    display: none;
  }
  .hero-copy {
    max-width: 220px;
  }
  .ticker {
    font-size: 8px;
    gap: 8px;
    padding: 12px 0;
  }
  .ticker > text:nth-child(2) {
    display: none;
  }
  .section-heading {
    margin: 23px 0 14px;
  }
  .section-title {
    font-size: 21px;
  }
  .section-number {
    font-size: 9px;
  }
  .eyebrow {
    font-size: 8px;
  }
  .category {
    padding: 8px 12px;
    font-size: 10px;
  }
  .category-row {
    padding-bottom: 16px;
  }
  .dish-grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
    gap: 13px;
  }
  .dish-art {
    height: 137px;
  }
  .dish-emoji {
    font-size: 68px;
  }
  .dish-tag {
    font-size: 8px;
    top: 9px;
    left: 9px;
  }
  .dish-art-word {
    font-size: 6px;
  }
  .dish-info {
    padding: 13px 11px;
  }
  .dish-name {
    font-size: 13px;
  }
  .dish-description {
    font-size: 9px;
    min-height: 32px;
    margin: 8px 0 10px;
  }
  .price {
    font-size: 21px;
  }
  .price text {
    font-size: 8px;
  }
  .add-button {
    width: 27px;
    height: 27px;
    line-height: 24px;
  }
  .footer {
    font-size: 7px;
    letter-spacing: 0;
    gap: 10px;
    padding-bottom: 110px;
  }
  .cart-bar {
    bottom: calc(14px + env(safe-area-inset-bottom));
    padding: 10px;
  }
  .cart-bar > button {
    font-size: 11px;
    padding: 14px 12px;
  }
  .cart-bar-price {
    font-size: 21px;
    margin-left: 13px;
  }
  .overlay {
    padding: 14px;
  }
  .modal {
    padding: 26px 22px;
    max-height: 88vh;
  }
  .order-card {
    padding: 17px;
  }
  .status-pill {
    font-size: 9px;
  }
  .order-items > view {
    font-size: 11px;
  }
  .order-items .muted {
    font-size: 9px;
  }
  .order-bottom .muted {
    font-size: 9px;
  }
  .guide-card {
    padding: 25px;
  }
  .guide-text {
    font-size: 12px;
  }
  .receipt-modal {
    padding: 32px 25px;
  }
  .page-heading {
    padding-top: 8px;
  }
}
@media (max-width: 360px) {
  .hero-title {
    font-size: 25px;
  }
  .hero-art {
    right: -32px;
    width: 145px;
  }
  .hero-copy {
    max-width: 190px;
  }
  .hero-foot > text:last-child {
    display: none;
  }
  .dish-name {
    font-size: 12px;
  }
}
</style>

<style>
@media (max-width: 760px) {
  .sidebar .nav-item {
    padding: 10px 3px;
    gap: 3px;
    font-size: 10px;
  }
  .sidebar .nav-item > text:first-child {
    font-size: 14px;
  }
}
</style>

<style>
.app-shell.has-player .cart-bar {
  bottom: calc(137px + env(safe-area-inset-bottom));
}
.app-shell.has-player .footer {
  padding-bottom: 235px;
}
</style>
