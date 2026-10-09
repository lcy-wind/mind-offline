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
  note: string;
  created_at: string;
  items: { name: string; emoji: string; quantity: number; mood: string }[];
};
const token = ref(sessionStorage.getItem("mind-offline-admin") || ""),
  password = ref(""),
  tab = ref("orders"),
  filter = ref("active"),
  error = ref(""),
  busy = ref(false),
  loading = ref(false),
  notice = ref("");
const orders = ref<Order[]>([]),
  dishes = ref<Dish[]>([]),
  summary = ref({ orders: 0, active: 0, points: 0 }),
  editing = ref<Dish | null>(null);
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
const visible = computed(() =>
  orders.value.filter(
    (o) =>
      filter.value === "all" ||
      (filter.value === "active"
        ? ["pending", "cooking", "ready"].includes(o.status)
        : o.status === filter.value),
  ),
);
let timer: ReturnType<typeof setInterval>;
async function api<T>(
  path: string,
  method = "GET",
  data?: unknown,
): Promise<T> {
  const res = await fetch("/api" + path, {
    method,
    headers: {
      "Content-Type": "application/json",
      Authorization: "Bearer " + token.value,
    },
    body: data ? JSON.stringify(data) : undefined,
  });
  const out = await res.json();
  if (!res.ok) {
    if (res.status === 401 && path !== "/admin/login") {
      token.value = "";
      sessionStorage.removeItem("mind-offline-admin");
    }
    throw Error(out.error || "请求失败");
  }
  return out;
}
async function load() {
  if (!token.value || loading.value) return;
  loading.value = true;
  try {
    const [os, ds, s] = await Promise.all([
      api<Order[]>("/admin/orders"),
      api<Dish[]>("/menu"),
      api<typeof summary.value>("/admin/summary"),
    ]);
    orders.value = os;
    dishes.value = ds;
    summary.value = s;
    error.value = "";
  } catch (e) {
    error.value = (e as Error).message;
  } finally {
    loading.value = false;
  }
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
    await load();
  } catch (e) {
    error.value = (e as Error).message;
  } finally {
    busy.value = false;
  }
}
async function logout() {
  try {
    await api("/admin/logout", "POST");
    token.value = "";
    sessionStorage.removeItem("mind-offline-admin");
  } catch (e) {
    error.value = (e as Error).message;
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
    await load();
    notice.value =
      status === "cancelled" ? "订单已取消，精神值已退回" : "订单状态已更新";
  } catch (e) {
    error.value = (e as Error).message;
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
    await load();
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
    await load();
  } catch (e) {
    error.value = (e as Error).message;
  } finally {
    busy.value = false;
  }
}
function date(s: string) {
  return new Date(s).toLocaleString("zh-CN", {
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
  });
}
onMounted(() => {
  load();
  timer = setInterval(() => {
    if (!busy.value && !editing.value && !document.hidden) load();
  }, 8000);
});
onUnmounted(() => clearInterval(timer));
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
        placeholder="请输入部署时生成的口令"
      />
      <p v-if="error" role="alert" class="error">{{ error }}</p>
      <button class="primary" :disabled="busy">
        {{ busy ? "正在开门…" : "进入工作台 →" }}</button
      ><a href="/" class="back-link">← 我是来吃饭的，回到菜单</a
      ><small>仅模拟点餐 · 不涉及真实交易</small>
    </form>
  </div>
  <div v-else class="admin-shell">
    <header>
      <a href="/" class="wordmark">精神离职 <small>MIND OFFLINE</small></a>
      <div class="header-right">
        <span class="staff-tag">店长工作台</span
        ><a href="/" target="_blank" rel="noopener">顾客视角 ↗</a
        ><button @click="logout">退出</button>
      </div>
    </header>
    <main>
      <div class="heading">
        <div>
          <span class="eyebrow">TODAY AT THE OFFLINE KITCHEN</span>
          <h1>今天也要，认真摸鱼。</h1>
          <p class="muted">接住每一份精神离职申请。数据每 8 秒自动刷新。</p>
        </div>
        <button class="outline" :disabled="loading" @click="load">
          {{ loading ? "更新中…" : "↻ 刷新" }}
        </button>
      </div>
      <div class="stats">
        <div>
          <span>今日离职申请</span
          ><strong>{{ summary.orders }}<small> 单</small></strong>
        </div>
        <div>
          <span>今日待处理</span
          ><strong>{{ summary.active }}<small> 单</small></strong>
        </div>
        <div>
          <span>今日精神值流水</span
          ><strong>{{ summary.points }}<small> 点</small></strong
          ><span class="stat-note">不含取消订单 · 无现金价值</span>
        </div>
      </div>
      <div class="tabs">
        <button :class="{ selected: tab === 'orders' }" @click="tab = 'orders'">
          订单工作台</button
        ><button :class="{ selected: tab === 'menu' }" @click="tab = 'menu'">
          菜单管理
        </button>
      </div>
      <p v-if="error" class="error" role="alert">{{ error }}</p>
      <div v-if="notice" class="notice" role="status" @click="notice = ''">
        {{ notice }} <span>×</span>
      </div>
      <section v-if="tab === 'orders'">
        <div class="filters">
          <button
            v-for="(name, key) in {
              active: '进行中',
              pending: '待接单',
              cooking: '制作中',
              ready: '待取餐',
              all: '全部订单',
            }"
            :key="key"
            :class="{ selected: filter === key }"
            @click="filter = key"
          >
            {{ name }}</button
          ><span>最近 100 笔</span>
        </div>
        <div v-if="!visible.length" class="empty">
          <span>☕</span>
          <h2>暂时没有{{ filter === "all" ? "订单" : "待处理的订单" }}</h2>
          <p>给自己倒杯水，或者去顾客端下一单试试。</p>
          <a href="/" target="_blank" rel="noopener">打开精神食堂 ↗</a>
        </div>
        <div class="order-grid">
          <article v-for="o in visible" :key="o.id" class="order-card">
            <div class="order-heading">
              <strong>MO-{{ String(o.number).padStart(4, "0") }}</strong
              ><span class="status" :class="o.status">{{
                labels[o.status]
              }}</span>
            </div>
            <p class="time">{{ date(o.created_at) }}</p>
            <div class="items">
              <div v-for="(i, k) in o.items" :key="k">
                <span class="food">{{ i.emoji }}</span>
                <div>
                  <strong>{{ i.name }}</strong
                  ><small>{{ i.mood }}</small>
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
      </section>
      <section v-else>
        <div class="menu-heading">
          <p class="muted">修改价格、编辑文案，或者让某道菜暂时精神离职。</p>
          <button class="primary" @click="edit()">+ 新增补给</button>
        </div>
        <div class="menu-list">
          <article v-for="d in dishes" :key="d.id" class="menu-row">
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
        </div>
      </section>
      <footer>精神离职 · 店长内部工作台 / 这是虚拟食堂，没有真实营收。</footer>
    </main>
  </div>
  <div v-if="editing" class="overlay" @click.self="!busy && (editing = null)">
    <form class="edit-modal" @submit.prevent="save">
      <div class="modal-heading">
        <h2>{{ editing.id ? "编辑精神补给" : "新增精神补给" }}</h2>
        <button type="button" :disabled="busy" @click="editing = null">
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
