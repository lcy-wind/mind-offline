export type PlayMode = "sequence" | "shuffle" | "single" | "loop";
export interface QueueOrder { order: number[]; position: number; mode: PlayMode; previousRound?: number[]; nextRound?: number[] }
export function createQueueOrder(count: number, current: number, mode: PlayMode, random = Math.random): QueueOrder {
  if (count <= 0 || current < 0 || current >= count) return { order: [], position: -1, mode };
  const order = Array.from({ length: count }, (_, i) => i);
  if (mode !== "shuffle") return { order, position: current, mode };
  const rest = order.filter(i => i !== current);
  for (let i = rest.length - 1; i > 0; i--) {
    const j = Math.floor(random() * (i + 1));
    [rest[i], rest[j]] = [rest[j], rest[i]];
  }
  return { order: [current, ...rest], position: 0, mode };
}
export function moveQueue(state: QueueOrder, direction: -1 | 1, ended = false, random = Math.random): QueueOrder | null {
  const count = state.order.length;
  if (!count || state.position < 0) return null;
  if (ended && state.mode === "single") return { ...state };
  const next = state.position + direction;
  if (next >= 0 && next < count) return { ...state, position: next };
  if (state.mode === "sequence") return null;
  if (state.mode === "shuffle") {
    if (direction === -1) {
      return state.previousRound ? {order: state.previousRound, position: count - 1, mode: "shuffle", nextRound: state.order} : null;
    }
    if (state.nextRound) return {order: state.nextRound, position: 0, mode: "shuffle", previousRound: state.order};
    const last = state.order[state.position];
    const first = count > 1 ? (last + 1 + Math.floor(random() * (count - 1))) % count : 0;
    const round = createQueueOrder(count, first, "shuffle", random);
    return { ...round, previousRound: state.order };
  }
  return { ...state, position: (next + count) % count };
}
