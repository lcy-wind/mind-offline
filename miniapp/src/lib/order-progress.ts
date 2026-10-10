interface TimedOrder { status: string; auto_started_at?: string | null; step_seconds?: number }
const stages = ["pending", "cooking", "ready", "completed"];
export function orderProgress(order: TimedOrder, now: number) {
  const rank = stages.indexOf(order.status);
  const started = Date.parse(order.auto_started_at || "");
  const step = (order.step_seconds || 30) * 1000;
  if (order.status === "cancelled") return { percent: 0, remaining: null, due: false, hint: "订单已取消" };
  if (order.status === "completed") return { percent: 100, remaining: null, due: false, hint: "自动出餐已完成，精神补给到账。" };
  if (rank < 0 || !Number.isFinite(started)) return { percent: Math.max(0, rank / 3 * 100), remaining: null, due: false, hint: "等待订单状态更新" };
  const remaining = Math.max(0, Math.ceil((started + (rank + 1) * step - now) / 1000));
  const percent = Math.round(Math.max(rank / 3 * 100, Math.min((rank + 1) / 3 * 100, (now - started) / (3 * step) * 100)));
  const names = ["进入制作中", "出锅，可取餐", "自动完成"];
  return { percent, remaining, due: remaining === 0, hint: remaining > 0 ? remaining + " 秒后" + names[rank] : "正在同步出餐进度…" };
}
