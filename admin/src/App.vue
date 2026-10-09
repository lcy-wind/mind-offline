<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted } from "vue";
type Dish = {
  id?: number;
  name: string;
  description: string;
  category: string;
  emoji: string;
  price: number;
  available: boolean;
};
type Order = {
  id: string;
  number: number;
  total: number;
  status: string;
  mood: string;
  note: string;
  quote: string;
  created_at: string;
  updated_at: string;
  customer_id: string;
  username: string;
  items: {
    name: string;
    emoji: string;
    quantity: number;
    mood: string;
    price: number;
  }[];
};
type Customer = {
  id: string;
  username: string;
  balance: number;
  disabled: boolean;
  admin_note: string;
  created_at: string;
  last_login_at: string | null;
  last_claim: string | null;
  order_count: number;
  completed_count: number;
  cancelled_count: number;
  active_count: number;
  total_points: number;
  last_order_at: string | null;
};
type Popular = { name: string; emoji: string; quantity: number };
type Detail = {
  customer: Customer;
  favorites: Popular[];
  favorite_mood: string;
};
type Page<T> = { items: T[]; total: number; page: number; page_size: number };
type Overview = {
  today_orders: number;
  total_orders: number;
  pending: number;
  cooking: number;
  ready: number;
  completed: number;
  cancelled: number;
  today_points: number;
  customers: number;
  new_customers: number;
  disabled_customers: number;
  available_dishes: number;
  trend: { date: string; orders: number; points: number }[];
  popular: Popular[];
};
const emptyOverview = (): Overview => ({
  today_orders: 0,
  total_orders: 0,
  pending: 0,
  cooking: 0,
  ready: 0,
  completed: 0,
  cancelled: 0,
  today_points: 0,
  customers: 0,
  new_customers: 0,
  disabled_customers: 0,
  available_dishes: 0,
  trend: [],
  popular: [],
});
const token = ref(sessionStorage.getItem("mind-offline-admin") || ""),
  password = ref(""),
  tab = ref("overview"),
  error = ref(""),
  notice = ref(""),
  busy = ref(false),
  loading = ref(false),
  updatedAt = ref("");
const overview = ref<Overview>(emptyOverview()),
  dishes = ref<Dish[]>([]),
  recentCustomers = ref<Customer[]>([]),
  editing = ref<Dish | null>(null);
const orderResult = ref<Page<Order>>({
    items: [],
    total: 0,
    page: 1,
    page_size: 20,
  }),
  orderPage = ref(1),
  orderStatus = ref("active"),
  orderSearch = ref(""),
  orderQuery = ref("");
const customerResult = ref<Page<Customer>>({
    items: [],
    total: 0,
    page: 1,
    page_size: 20,
  }),
  customerPage = ref(1),
  customerStatus = ref("all"),
  customerSearch = ref(""),
  customerQuery = ref("");
const selectedId = ref(""),
  detail = ref<Detail | null>(null),
  detailOrders = ref<Page<Order>>({
    items: [],
    total: 0,
    page: 1,
    page_size: 20,
  }),
  detailPage = ref(1),
  detailLoading = ref(false),
  detailError = ref(""),
  noteDraft = ref("");
const dishSearch = ref(""),
  dishStatus = ref("all");
const categories = ["续命主食", "精神饮品", "摸鱼小食", "离职套餐"];
const labels: Record<string, string> = {
  pending: "待接单",
  cooking: "制作中",
  ready: "待取餐",
  completed: "已完成",
  cancelled: "已取消",
};
const next: Record<string, { status: string; label: string }> = {
  pending: { status: "cooking", label: "接单 · 开始制作" },
  cooking: { status: "ready", label: "出餐 · 可以取餐" },
  ready: { status: "completed", label: "确认完成" },
};
const sections = [
  { id: "overview", icon: "◈", name: "经营概览", en: "OVERVIEW" },
  { id: "orders", icon: "≡", name: "订单管理", en: "ORDERS" },
  { id: "customers", icon: "◎", name: "顾客档案", en: "CUSTOMERS" },
  { id: "menu", icon: "✳", name: "菜单管理", en: "MENU" },
];
const currentSection = computed(
  () => sections.find((s) => s.id === tab.value)!,
);
const openOrders = computed(
  () => overview.value.pending + overview.value.cooking + overview.value.ready,
);
const chartMax = computed(() =>
  Math.max(1, ...overview.value.trend.map((d) => d.orders)),
);
const filteredDishes = computed(() =>
  dishes.value.filter(
    (d) =>
      (!dishSearch.value ||
        d.name.includes(dishSearch.value) ||
        d.description.includes(dishSearch.value)) &&
      (dishStatus.value === "all" ||
        d.available === (dishStatus.value === "available")),
  ),
);
let timer: ReturnType<typeof setInterval>;
let orderSeq = 0,
  customerSeq = 0,
  detailSeq = 0,
  refreshSeq = 0;
function clearPrivate() {
  token.value = "";
  sessionStorage.removeItem("mind-offline-admin");
  overview.value = emptyOverview();
  orderResult.value.items = [];
  customerResult.value.items = [];
  recentCustomers.value = [];
  closeDetail();
  editing.value = null;
  notice.value = "";
}
async function api<T>(
  path: string,
  method = "GET",
  data?: unknown,
): Promise<T> {
  const captured = token.value;
  const res = await fetch("/api" + path, {
    method,
    headers: {
      "Content-Type": "application/json",
      Authorization: "Bearer " + captured,
    },
    body: data ? JSON.stringify(data) : undefined,
  });
  const out = await res.json();
  if (captured !== token.value) throw Error("登录状态已变化，请重新操作");
  if (!res.ok) {
    if (res.status === 401 && path !== "/admin/login") clearPrivate();
    throw Error(out.error || "请求失败");
  }
  return out;
}
async function loadOrders() {
  const seq = ++orderSeq;
  const data = await api<Page<Order>>(
    "/admin/order-list?" +
      new URLSearchParams({
        page: String(orderPage.value),
        status: orderStatus.value,
        q: orderQuery.value,
      }),
  );
  if (seq === orderSeq) orderResult.value = data;
}
async function loadCustomers() {
  const seq = ++customerSeq;
  const data = await api<Page<Customer>>(
    "/admin/customers?" +
      new URLSearchParams({
        page: String(customerPage.value),
        status: customerStatus.value,
        q: customerQuery.value,
      }),
  );
  if (seq === customerSeq) customerResult.value = data;
}
async function refresh() {
  if (!token.value) return;
  const seq = ++refreshSeq;
  loading.value = true;
  try {
    const tasks: Promise<unknown>[] = [
      api<Overview>("/admin/overview").then((v) => {
        if (seq === refreshSeq) overview.value = v;
      }),
    ];
    if (tab.value === "overview")
      tasks.push(
        api<Page<Customer>>("/admin/customers").then((v) => {
          if (seq === refreshSeq) recentCustomers.value = v.items.slice(0, 5);
        }),
      );
    if (tab.value === "orders") tasks.push(loadOrders());
    if (tab.value === "customers") tasks.push(loadCustomers());
    if (tab.value === "menu")
      tasks.push(
        api<Dish[]>("/menu").then((v) => {
          if (seq === refreshSeq) dishes.value = v;
        }),
      );
    await Promise.all(tasks);
    if (seq === refreshSeq) {
      error.value = "";
      updatedAt.value = new Date().toLocaleTimeString("zh-CN", {
        hour12: false,
      });
    }
  } catch (e) {
    if (seq === refreshSeq) error.value = (e as Error).message;
  } finally {
    if (seq === refreshSeq) loading.value = false;
  }
}
function navigate(id: string) {
  tab.value = id;
  notice.value = "";
  error.value = "";
  refresh();
}
function searchOrders() {
  orderQuery.value = orderSearch.value.trim();
  orderPage.value = 1;
  refresh();
}
function setOrderStatus(s: string) {
  orderStatus.value = s;
  orderPage.value = 1;
  refresh();
}
function changeOrderPage(n: number) {
  orderPage.value += n;
  refresh();
}
function searchCustomers() {
  customerQuery.value = customerSearch.value.trim();
  customerPage.value = 1;
  refresh();
}
function changeCustomerPage(n: number) {
  customerPage.value += n;
  refresh();
}
function showOrders(status = "active") {
  orderStatus.value = status;
  orderPage.value = 1;
  orderSearch.value = "";
  orderQuery.value = "";
  navigate("orders");
}
async function login() {
  if (busy.value) return;
  busy.value = true;
  error.value = "";
  try {
    const s = await api<{ token: string }>("/admin/login", "POST", {
      password: password.value,
    });
    token.value = s.token;
    sessionStorage.setItem("mind-offline-admin", s.token);
    password.value = "";
    await refresh();
  } catch (e) {
    error.value = (e as Error).message;
  } finally {
    busy.value = false;
  }
}
async function logout() {
  if (busy.value) return;
  busy.value = true;
  try {
    await api("/admin/logout", "POST");
    clearPrivate();
    error.value = "";
  } catch (e) {
    error.value = (e as Error).message;
  } finally {
    busy.value = false;
  }
}
async function update(o: Order, status: string) {
  if (busy.value) return;
  if (
    status === "cancelled" &&
    !confirm("取消这笔订单？精神值将自动退回顾客。")
  )
    return;
  busy.value = true;
  try {
    await api("/admin/orders/" + o.id, "PATCH", { status });
    await refresh();
    if (selectedId.value) await loadDetail(true);
    notice.value =
      status === "cancelled" ? "订单已取消，精神值已退回" : "订单状态已更新";
  } catch (e) {
    error.value = (e as Error).message;
    detailError.value = error.value;
  } finally {
    busy.value = false;
  }
}
function edit(d?: Dish) {
  editing.value = d
    ? { ...d }
    : {
        name: "",
        description: "",
        category: "续命主食",
        emoji: "🍜",
        price: 20,
        available: true,
      };
}
async function save() {
  if (!editing.value || busy.value) return;
  busy.value = true;
  try {
    const d = editing.value;
    await api(
      "/admin/dishes" + (d.id ? "/" + d.id : ""),
      d.id ? "PUT" : "POST",
      d,
    );
    editing.value = null;
    await refresh();
    notice.value = "菜单已更新";
  } catch (e) {
    error.value = (e as Error).message;
  } finally {
    busy.value = false;
  }
}
async function toggle(d: Dish) {
  if (busy.value) return;
  busy.value = true;
  try {
    await api("/admin/dishes/" + d.id, "PUT", {
      ...d,
      available: !d.available,
    });
    await refresh();
  } catch (e) {
    error.value = (e as Error).message;
  } finally {
    busy.value = false;
  }
}
function closeDetail() {
  selectedId.value = "";
  detail.value = null;
  detailSeq++;
  detailLoading.value = false;
  detailError.value = "";
  noteDraft.value = "";
}
async function openCustomer(id: string) {
  if (!id) return;
  closeDetail();
  selectedId.value = id;
  detailPage.value = 1;
  detailOrders.value = { items: [], total: 0, page: 1, page_size: 20 };
  await loadDetail();
}
async function loadDetail(keepNote = false) {
  const id = selectedId.value;
  if (!id) return;
  const seq = ++detailSeq;
  detailLoading.value = true;
  try {
    const [d, os] = await Promise.all([
      api<Detail>("/admin/customers/" + id),
      api<Page<Order>>(
        "/admin/order-list?" +
          new URLSearchParams({
            customer_id: id,
            page: String(detailPage.value),
            status: "all",
          }),
      ),
    ]);
    if (seq !== detailSeq) return;
    detail.value = d;
    detailOrders.value = os;
    if (!keepNote) noteDraft.value = d.customer.admin_note;
    detailError.value = "";
  } catch (e) {
    if (seq === detailSeq) detailError.value = (e as Error).message;
  } finally {
    if (seq === detailSeq) detailLoading.value = false;
  }
}
function changeDetailPage(n: number) {
  detailPage.value += n;
  loadDetail(true);
}
async function saveNote() {
  if (!detail.value || busy.value) return;
  busy.value = true;
  try {
    await api("/admin/customers/" + selectedId.value, "PATCH", {
      admin_note: noteDraft.value,
    });
    await loadDetail();
    notice.value = "店长备注已保存";
    await refresh();
  } catch (e) {
    detailError.value = (e as Error).message;
  } finally {
    busy.value = false;
  }
}
async function toggleCustomer() {
  if (!detail.value || busy.value) return;
  const c = detail.value.customer;
  const disabled = !c.disabled;
  if (
    !confirm(
      disabled
        ? "停用 @" + c.username + "？该顾客会退出登录，已有订单和余额仍保留。"
        : "恢复 @" + c.username + "？恢复后顾客需要重新登录。",
    )
  )
    return;
  busy.value = true;
  try {
    await api("/admin/customers/" + c.id, "PATCH", { disabled });
    await loadDetail(true);
    await refresh();
    notice.value = disabled ? "顾客账号已停用" : "顾客账号已恢复";
  } catch (e) {
    detailError.value = (e as Error).message;
  } finally {
    busy.value = false;
  }
}
function orderNumber(o: Order) {
  return "MO-" + String(o.number).padStart(4, "0");
}
function date(s: string | null, full = false) {
  if (!s) return "暂无记录";
  return new Date(s).toLocaleString("zh-CN", {
    ...(full ? { year: "numeric" as const } : {}),
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
    hour12: false,
  });
}
function escape(event: KeyboardEvent) {
  if (event.key === "Escape" && !busy.value) {
    if (editing.value) editing.value = null;
    else closeDetail();
  }
}
onMounted(() => {
  refresh();
  timer = setInterval(() => {
    if (!busy.value && !loading.value && !editing.value && !document.hidden)
      refresh();
  }, 8000);
  document.addEventListener("keydown", escape);
});
onUnmounted(() => {
  clearInterval(timer);
  document.removeEventListener("keydown", escape);
});
</script>

<template>
  <div v-if="!token" class="login-page">
    <div class="login-art">
      <span class="wordmark">精神离职 / MIND OFFLINE</span>
      <h1>店长可以摸鱼，<br /><em>但别忘了出餐。</em></h1>
      <div class="big-symbol">✳</div>
      <p>这里没有绩效考核，只有等待快乐的打工人。</p>
    </div>
    <form class="login-card" @submit.prevent="login">
      <span class="eyebrow">STAFF ONLY / 内部通道</span>
      <h2>欢迎回到食堂</h2>
      <p class="muted">输入店长口令，开启今日精神补给。</p>
      <label for="password">店长口令</label
      ><input
        id="password"
        v-model="password"
        type="password"
        autocomplete="current-password"
        required
        placeholder="请输入店长口令"
      />
      <p v-if="error" role="alert" class="error">{{ error }}</p>
      <button class="primary" :disabled="busy">
        {{ busy ? "正在开门…" : "进入工作台 →" }}</button
      ><a href="/" class="back-link">← 我是来吃饭的，回到菜单</a
      ><small>仅模拟点餐 · 不涉及真实交易</small>
    </form>
  </div>
  <div v-else class="workspace">
    <aside class="sidebar">
      <a href="/" class="brand"
        ><span class="brand-tile">离</span
        ><span>精神离职<small>MIND OFFLINE</small></span></a
      >
      <div class="sidebar-caption">店长的摸鱼控制台</div>
      <nav aria-label="管理导航">
        <button
          v-for="s in sections"
          :key="s.id"
          :class="{ active: tab === s.id }"
          @click="navigate(s.id)"
        >
          <span class="nav-symbol">{{ s.icon }}</span
          ><span
            >{{ s.name }}<small>{{ s.en }}</small></span
          ><b v-if="s.id === 'orders' && openOrders">{{ openOrders }}</b>
        </button>
      </nav>
      <div class="sidebar-note">
        <span>✳</span>
        <p>认真接单，<br />合理离线。</p>
        <small>虚拟食堂 · 快乐营业</small>
      </div>
      <a class="customer-link" href="/" target="_blank" rel="noopener"
        >打开顾客端 ↗</a
      ><button class="sidebar-logout" :disabled="busy" @click="logout">
        退出店长账号
      </button>
    </aside>
    <div class="workspace-main">
      <header class="workspace-header">
        <div>
          <span class="eyebrow">WORKSPACE / {{ currentSection.en }}</span>
          <h1>{{ currentSection.name }}</h1>
        </div>
        <div class="header-tools">
          <span class="updated">{{
            updatedAt ? "更新于 " + updatedAt : "正在加载…"
          }}</span
          ><button class="outline" :disabled="loading" @click="refresh">
            {{ loading ? "更新中…" : "↻ 刷新数据" }}</button
          ><span class="manager-avatar">店</span>
        </div>
      </header>
      <main class="workspace-content">
        <p v-if="error" class="error" role="alert">{{ error }}</p>
        <div v-if="notice" class="notice" role="status">
          {{ notice
          }}<button aria-label="关闭提示" @click="notice = ''">×</button>
        </div>
        <template v-if="tab === 'overview'">
          <section class="welcome-banner">
            <div>
              <span class="eyebrow">OFFLINE KITCHEN / 今日营业中</span>
              <h2>每一份订单，<br />都是一次精神补给。</h2>
              <p>顾客的快乐进度，就交给你了。</p>
            </div>
            <div class="welcome-sticker">
              店长今日任务<br /><strong>接住快乐</strong
              ><small>NO KPI, JUST GOOD FOOD.</small>
            </div>
            <span class="welcome-star">✳</span>
          </section>
          <section class="metric-grid" aria-label="经营指标">
            <button class="metric lavender" @click="navigate('customers')">
              <span>注册顾客 <i>◎</i></span
              ><strong>{{ overview.customers }}<small>人</small></strong
              ><em>今日新注册 {{ overview.new_customers }} 人</em></button
            ><button class="metric" @click="showOrders('all')">
              <span>今日订单 <i>↗</i></span
              ><strong>{{ overview.today_orders }}<small>单</small></strong
              ><em>累计 {{ overview.total_orders }} 笔离职申请</em></button
            ><button class="metric lime" @click="showOrders('active')">
              <span>待处理订单 <i>≡</i></span
              ><strong>{{ openOrders }}<small>单</small></strong
              ><em>包含之前日期尚未完成的订单</em>
            </button>
            <div class="metric">
              <span>今日精神值流水 <i>✦</i></span
              ><strong>{{ overview.today_points }}<small>点</small></strong
              ><em>不含取消订单 · 非真实营收</em>
            </div>
          </section>
          <section class="queue-strip">
            <span>当前出餐队列</span
            ><button @click="showOrders('pending')">
              <i class="dot purple"></i>待接单
              <b>{{ overview.pending }}</b></button
            ><button @click="showOrders('cooking')">
              <i class="dot orange"></i>制作中
              <b>{{ overview.cooking }}</b></button
            ><button @click="showOrders('ready')">
              <i class="dot green"></i>待取餐
              <b>{{ overview.ready }}</b></button
            ><button class="queue-link" @click="showOrders()">
              进入工作台 →
            </button>
          </section>
          <div class="dashboard-grid">
            <section class="panel">
              <div class="panel-heading">
                <div>
                  <span class="eyebrow">LAST 7 DAYS</span>
                  <h2>精神离职趋势</h2>
                </div>
                <span class="muted">订单数 · 北京时间</span>
              </div>
              <div class="trend-chart">
                <div
                  v-for="d in overview.trend"
                  :key="d.date"
                  class="chart-day"
                  :title="
                    d.date + '：' + d.orders + '单，' + d.points + '精神值'
                  "
                >
                  <span>{{ d.orders }}</span>
                  <div class="bar-track">
                    <div
                      class="chart-bar"
                      :class="{ zero: d.orders === 0 }"
                      :style="{
                        height: Math.max(2, (d.orders / chartMax) * 100) + '%',
                      }"
                    ></div>
                  </div>
                  <small>{{ d.date.slice(5).replace("-", "/") }}</small>
                </div>
              </div>
              <p class="chart-caption">
                订单数含取消单；流水按未取消订单统计。
              </p>
            </section>
            <section class="panel">
              <div class="panel-heading">
                <div>
                  <span class="eyebrow">COMFORT FOOD RANKING</span>
                  <h2>本周续命榜</h2>
                </div>
                <span class="muted">近7日 / 份数</span>
              </div>
              <div v-if="!overview.popular.length" class="compact-empty">
                还没有补给记录，等第一位打工人来。
              </div>
              <div
                v-for="(d, i) in overview.popular"
                :key="d.name + d.emoji"
                class="ranking-row"
              >
                <span class="rank">{{ String(i + 1).padStart(2, "0") }}</span
                ><span class="ranking-emoji">{{ d.emoji }}</span>
                <div>
                  <strong>{{ d.name }}</strong>
                  <div class="ranking-track">
                    <i
                      :style="{
                        width:
                          (d.quantity / overview.popular[0].quantity) * 100 +
                          '%',
                      }"
                    ></i>
                  </div>
                </div>
                <b>{{ d.quantity }}<small> 份</small></b>
              </div>
            </section>
          </div>
          <section class="panel">
            <div class="panel-heading">
              <div>
                <span class="eyebrow">NEW FACES AT THE CANTEEN</span>
                <h2>最近加入的打工人</h2>
              </div>
              <button class="text-button" @click="navigate('customers')">
                全部顾客 →
              </button>
            </div>
            <div v-if="!recentCustomers.length" class="compact-empty">
              暂无注册顾客，顾客注册后会自动出现在这里。
            </div>
            <div class="recent-grid">
              <button
                v-for="c in recentCustomers"
                :key="c.id"
                class="recent-person"
                @click="openCustomer(c.id)"
              >
                <span class="person-avatar">{{
                  c.username.slice(0, 1).toUpperCase()
                }}</span
                ><span
                  ><strong>@{{ c.username }}</strong
                  ><small>{{ date(c.created_at) }} 加入</small></span
                ><b>↗</b>
              </button>
            </div>
          </section>
        </template>
        <section v-if="tab === 'orders'">
          <div class="section-intro">
            <div>
              <h2>接住每一份精神离职申请。</h2>
              <p>从接单到出餐，顾客端会同步显示订单进度。</p>
            </div>
            <span class="count-badge">{{ orderResult.total }} 笔匹配订单</span>
          </div>
          <form class="search-toolbar" @submit.prevent="searchOrders">
            <div class="search-field">
              <span>⌕</span
              ><input
                v-model="orderSearch"
                aria-label="搜索订单"
                placeholder="搜索顾客用户名或订单号，如 MO-0001"
                maxlength="80"
              />
            </div>
            <button class="primary">搜索订单</button>
          </form>
          <div class="filters">
            <button
              v-for="(name, key) in {
                active: '进行中',
                pending: '待接单',
                cooking: '制作中',
                ready: '待取餐',
                completed: '已完成',
                cancelled: '已取消',
                all: '全部订单',
              }"
              :key="key"
              :class="{ selected: orderStatus === key }"
              @click="setOrderStatus(key)"
            >
              {{ name }}
            </button>
          </div>
          <div v-if="!orderResult.items.length && !loading" class="empty">
            <span>☕</span>
            <h2>这里暂时没有订单</h2>
            <p>换个搜索条件，或者给自己倒杯水。</p>
          </div>
          <div class="order-grid">
            <article
              v-for="o in orderResult.items"
              :key="o.id"
              class="order-card"
            >
              <div class="order-heading">
                <strong>{{ orderNumber(o) }}</strong
                ><span class="status" :class="o.status">{{
                  labels[o.status]
                }}</span>
              </div>
              <button
                v-if="o.customer_id"
                class="order-customer"
                @click="openCustomer(o.customer_id)"
              >
                ◎ @{{ o.username }} <span>查看顾客 ↗</span>
              </button>
              <p v-else class="legacy-label">历史访客 · 尚未绑定账号</p>
              <p class="time">{{ date(o.created_at) }} · {{ o.mood }}</p>
              <div class="items">
                <div v-for="(i, k) in o.items" :key="k">
                  <span class="food">{{ i.emoji }}</span>
                  <div>
                    <strong>{{ i.name }}</strong
                    ><small>{{ i.mood }} · {{ i.price }} 点</small>
                  </div>
                  <b>×{{ i.quantity }}</b>
                </div>
              </div>
              <p v-if="o.note" class="note">顾客说：{{ o.note }}</p>
              <div class="order-total">
                精神支出 <b>{{ o.total }} 点</b>
              </div>
              <div v-if="next[o.status]" class="actions">
                <button
                  v-if="['pending', 'cooking'].includes(o.status)"
                  class="cancel"
                  :disabled="busy"
                  @click="update(o, 'cancelled')"
                >
                  取消</button
                ><button
                  class="primary"
                  :disabled="busy"
                  @click="update(o, next[o.status].status)"
                >
                  {{ next[o.status].label }} →
                </button>
              </div>
              <p v-else class="closed-note">
                {{
                  o.status === "cancelled"
                    ? "精神值已原路退回"
                    : "今日快乐已送达 ✓"
                }}
              </p>
            </article>
          </div>
          <div class="pagination">
            <span>共 {{ orderResult.total }} 笔 · 第 {{ orderPage }} 页</span
            ><button
              class="outline"
              :disabled="orderPage <= 1 || loading"
              @click="changeOrderPage(-1)"
            >
              上一页</button
            ><button
              class="outline"
              :disabled="
                orderPage * orderResult.page_size >= orderResult.total ||
                loading
              "
              @click="changeOrderPage(1)"
            >
              下一页
            </button>
          </div>
        </section>
        <section v-if="tab === 'customers'">
          <div class="section-intro">
            <div>
              <h2>每一位打工人，都有自己的档案。</h2>
              <p>查看真实注册资料、精神余额与点餐偏好。</p>
            </div>
            <span class="count-badge"
              >{{ overview.customers }} 位顾客 /
              {{ overview.disabled_customers }} 位停用</span
            >
          </div>
          <form class="search-toolbar" @submit.prevent="searchCustomers">
            <div class="search-field">
              <span>⌕</span
              ><input
                v-model="customerSearch"
                aria-label="搜索顾客"
                placeholder="输入用户名或完整顾客 ID"
                maxlength="80"
              />
            </div>
            <select
              v-model="customerStatus"
              aria-label="账号状态"
              @change="searchCustomers"
            >
              <option value="all">全部账号</option>
              <option value="enabled">正常账号</option>
              <option value="disabled">已停用</option></select
            ><button class="primary">查找顾客</button>
          </form>
          <div class="table-container">
            <table class="customers-table">
              <thead>
                <tr>
                  <th>顾客</th>
                  <th>精神余额</th>
                  <th>订单 / 累计消费</th>
                  <th>注册时间</th>
                  <th>最近登录</th>
                  <th>状态</th>
                  <th></th>
                </tr>
              </thead>
              <tbody>
                <tr v-for="c in customerResult.items" :key="c.id">
                  <td>
                    <button class="table-person" @click="openCustomer(c.id)">
                      <span class="person-avatar">{{
                        c.username[0].toUpperCase()
                      }}</span
                      ><span
                        ><strong>{{ c.username }}</strong
                        ><small>ID {{ c.id.slice(0, 8) }}…</small></span
                      >
                    </button>
                  </td>
                  <td>
                    <b>{{ c.balance }}</b
                    ><small class="unit"> 点</small>
                  </td>
                  <td>
                    <strong>{{ c.order_count }} 单</strong
                    ><small class="subline">{{ c.total_points }} 精神值</small>
                  </td>
                  <td>{{ date(c.created_at) }}</td>
                  <td>{{ date(c.last_login_at) }}</td>
                  <td>
                    <span
                      class="account-status"
                      :class="{ disabled: c.disabled }"
                      >{{ c.disabled ? "已停用" : "正常" }}</span
                    >
                  </td>
                  <td>
                    <button class="text-button" @click="openCustomer(c.id)">
                      查看档案 ↗
                    </button>
                  </td>
                </tr>
                <tr v-if="!customerResult.items.length">
                  <td colspan="7" class="table-empty">
                    {{ loading ? "正在查找顾客…" : "暂无符合条件的顾客" }}
                  </td>
                </tr>
              </tbody>
            </table>
          </div>
          <div class="pagination">
            <span
              >共 {{ customerResult.total }} 位 · 第 {{ customerPage }} 页</span
            ><button
              class="outline"
              :disabled="customerPage <= 1 || loading"
              @click="changeCustomerPage(-1)"
            >
              上一页</button
            ><button
              class="outline"
              :disabled="
                customerPage * customerResult.page_size >=
                  customerResult.total || loading
              "
              @click="changeCustomerPage(1)"
            >
              下一页
            </button>
          </div>
        </section>
        <section v-if="tab === 'menu'">
          <div class="section-intro">
            <div>
              <h2>今天的快乐，由你定价。</h2>
              <p>
                {{ dishes.length }} 款补给 ·
                {{ dishes.filter((d) => d.available).length }} 款正在供应
              </p>
            </div>
            <button class="primary" @click="edit()">+ 新增补给</button>
          </div>
          <div class="search-toolbar">
            <div class="search-field">
              <span>⌕</span
              ><input
                v-model="dishSearch"
                placeholder="搜索菜品名称或文案"
                aria-label="搜索菜品"
              />
            </div>
            <select v-model="dishStatus" aria-label="供应状态">
              <option value="all">全部菜品</option>
              <option value="available">供应中</option>
              <option value="soldout">已售罄</option>
            </select>
          </div>
          <div class="menu-list">
            <article v-for="d in filteredDishes" :key="d.id" class="menu-row">
              <span class="dish-icon">{{ d.emoji }}</span>
              <div class="dish-copy">
                <strong>{{ d.name }}</strong>
                <p>{{ d.description }}</p>
                <small>{{ d.category }} · {{ d.price }} 精神值</small>
              </div>
              <span class="availability" :class="{ off: !d.available }">{{
                d.available ? "供应中" : "已售罄"
              }}</span>
              <div class="row-actions">
                <button class="outline" :disabled="busy" @click="toggle(d)">
                  {{ d.available ? "设为售罄" : "恢复供应" }}</button
                ><button class="outline" @click="edit(d)">编辑</button>
              </div>
            </article>
            <div v-if="!filteredDishes.length" class="empty">
              暂无符合条件的菜品。
            </div>
          </div>
        </section>
        <footer>
          MIND OFFLINE / 店长内部工作台
          <span>这里的流水是精神值，快乐才是硬通货。</span>
        </footer>
      </main>
    </div>
  </div>
  <div
    v-if="token && selectedId"
    class="drawer-backdrop"
    @click.self="!busy && closeDetail()"
  >
    <aside
      class="customer-drawer"
      role="dialog"
      aria-modal="true"
      aria-label="顾客档案"
    >
      <div class="drawer-heading">
        <span class="eyebrow">CUSTOMER FILE / 顾客档案</span
        ><button
          aria-label="关闭顾客详情"
          :disabled="busy"
          @click="closeDetail"
        >
          ×
        </button>
      </div>
      <p v-if="detailError" class="error" role="alert">{{ detailError }}</p>
      <div v-if="!detail" class="compact-empty">
        {{ detailLoading ? "正在打开顾客档案…" : "暂时无法读取资料"
        }}<button
          v-if="!detailLoading"
          class="text-button"
          @click="loadDetail()"
        >
          重试
        </button>
      </div>
      <template v-if="detail"
        ><div class="profile-hero">
          <span class="person-avatar large">{{
            detail.customer.username[0].toUpperCase()
          }}</span>
          <div>
            <h2>@{{ detail.customer.username }}</h2>
            <span
              class="account-status"
              :class="{ disabled: detail.customer.disabled }"
              >{{ detail.customer.disabled ? "账号已停用" : "账号正常" }}</span
            >
          </div>
          <button class="outline" :disabled="busy" @click="toggleCustomer">
            {{ detail.customer.disabled ? "恢复账号" : "停用账号" }}
          </button>
        </div>
        <div class="profile-id">
          顾客 ID <code>{{ detail.customer.id }}</code>
        </div>
        <div class="profile-metrics">
          <div>
            <span>精神余额</span><strong>{{ detail.customer.balance }}</strong>
          </div>
          <div>
            <span>累计订单</span
            ><strong>{{ detail.customer.order_count }}</strong>
          </div>
          <div>
            <span>累计消费</span
            ><strong>{{ detail.customer.total_points }}</strong>
          </div>
        </div>
        <dl class="profile-facts">
          <div>
            <dt>注册时间</dt>
            <dd>{{ date(detail.customer.created_at, true) }}</dd>
          </div>
          <div>
            <dt>最近登录</dt>
            <dd>{{ date(detail.customer.last_login_at, true) }}</dd>
          </div>
          <div>
            <dt>最近下单</dt>
            <dd>{{ date(detail.customer.last_order_at, true) }}</dd>
          </div>
          <div>
            <dt>最近领取补给</dt>
            <dd>{{ detail.customer.last_claim || "尚未领取" }}</dd>
          </div>
          <div>
            <dt>订单分布</dt>
            <dd>
              进行中 {{ detail.customer.active_count }} · 完成
              {{ detail.customer.completed_count }} · 取消
              {{ detail.customer.cancelled_count }}
            </dd>
          </div>
          <div>
            <dt>常选精神状态</dt>
            <dd>{{ detail.favorite_mood || "还没有偏好记录" }}</dd>
          </div>
        </dl>
        <div class="detail-section">
          <h3>常点的精神食粮</h3>
          <div class="favorite-chips">
            <span v-for="d in detail.favorites" :key="d.name + d.emoji"
              >{{ d.emoji }} {{ d.name }} <b>×{{ d.quantity }}</b></span
            >
            <p v-if="!detail.favorites.length" class="muted">
              这位打工人还没开始补给。
            </p>
          </div>
        </div>
        <form class="detail-section" @submit.prevent="saveNote">
          <div class="panel-heading">
            <h3>店长备注</h3>
            <small class="muted">仅店长可见 · {{ noteDraft.length }}/500</small>
          </div>
          <textarea
            v-model="noteDraft"
            maxlength="500"
            rows="3"
            placeholder="记录偏好或需要留意的事情，例如：喜欢灵魂离线套餐。"
            aria-label="店长备注"
          />
          <div class="note-actions">
            <span class="muted">停用不会删除订单或余额。</span
            ><button
              class="primary"
              :disabled="busy || noteDraft === detail.customer.admin_note"
            >
              {{ busy ? "处理中…" : "保存备注" }}
            </button>
          </div>
        </form>
        <div class="detail-section">
          <div class="panel-heading">
            <h3>历史订单</h3>
            <span class="muted">共 {{ detailOrders.total }} 笔</span>
          </div>
          <div v-if="!detailOrders.items.length" class="compact-empty">
            暂无历史订单
          </div>
          <details
            v-for="o in detailOrders.items"
            :key="o.id"
            class="history-order"
          >
            <summary>
              <span
                ><strong>{{ orderNumber(o) }}</strong
                ><small>{{ date(o.created_at) }}</small></span
              ><span class="status" :class="o.status">{{
                labels[o.status]
              }}</span
              ><b>{{ o.total }} 点</b><span>⌄</span>
            </summary>
            <div class="history-content">
              <div v-for="(i, k) in o.items" :key="k" class="history-line">
                <span
                  >{{ i.emoji }} {{ i.name }}<small>{{ i.mood }}</small></span
                ><span>×{{ i.quantity }} · {{ i.price * i.quantity }} 点</span>
              </div>
              <p v-if="o.note" class="note">顾客说：{{ o.note }}</p>
              <p class="muted">最后更新 {{ date(o.updated_at, true) }}</p>
              <div v-if="next[o.status]" class="actions">
                <button
                  v-if="['pending', 'cooking'].includes(o.status)"
                  class="cancel"
                  :disabled="busy"
                  @click="update(o, 'cancelled')"
                >
                  取消订单</button
                ><button
                  class="primary"
                  :disabled="busy"
                  @click="update(o, next[o.status].status)"
                >
                  {{ next[o.status].label }}
                </button>
              </div>
            </div>
          </details>
          <div v-if="detailOrders.total > 20" class="pagination">
            <span>第 {{ detailPage }} 页</span
            ><button
              class="outline"
              :disabled="detailPage <= 1 || detailLoading"
              @click="changeDetailPage(-1)"
            >
              上一页</button
            ><button
              class="outline"
              :disabled="detailPage * 20 >= detailOrders.total || detailLoading"
              @click="changeDetailPage(1)"
            >
              下一页
            </button>
          </div>
        </div>
        <p class="profile-footnote">
          当前仅收集用户名。手机号、地址等信息未收集；密码和登录凭证不会在后台展示。最近登录从本次升级后开始记录。消费与常点菜品不含已取消订单。
        </p></template
      >
    </aside>
  </div>
  <div
    v-if="token && editing"
    class="overlay"
    @click.self="!busy && (editing = null)"
  >
    <form
      class="edit-modal"
      role="dialog"
      aria-modal="true"
      aria-label="编辑菜品"
      @submit.prevent="save"
    >
      <div class="modal-heading">
        <h2>{{ editing.id ? "编辑精神补给" : "新增精神补给" }}</h2>
        <button
          type="button"
          :disabled="busy"
          aria-label="关闭菜品编辑"
          @click="editing = null"
        >
          ×
        </button>
      </div>
      <label
        >菜品名称<input v-model="editing.name" maxlength="30" required /></label
      ><label
        >抽象文案<textarea
          v-model="editing.description"
          maxlength="120"
          rows="3"
        />
      </label>
      <div class="form-row">
        <label
          >分类<select v-model="editing.category">
            <option v-for="c in categories" :key="c">{{ c }}</option>
          </select></label
        ><label
          >图标（Emoji）<input v-model="editing.emoji" maxlength="12" required
        /></label>
      </div>
      <label
        >精神值（1—200）<input
          v-model.number="editing.price"
          type="number"
          min="1"
          max="200"
          step="1"
          required /></label
      ><label class="checkbox"
        ><input v-model="editing.available" type="checkbox" />正在供应</label
      >
      <p v-if="error" class="error">{{ error }}</p>
      <button class="primary" :disabled="busy">
        {{ busy ? "保存中…" : "保存菜单" }}
      </button>
    </form>
  </div>
</template>
