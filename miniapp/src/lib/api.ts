const HOST = "https://mind-offline.duckdns.org";
let base = HOST;
// #ifdef H5
base = "";
// #endif
export interface Dish {
  id: number;
  name: string;
  description: string;
  category: string;
  emoji: string;
  price: number;
  available: boolean;
}
export interface Guest {
  id: string;
  username: string;
  balance: number;
  claimed_today: boolean;
}
export interface Line {
  dish_id: number;
  name?: string;
  emoji?: string;
  price?: number;
  quantity: number;
  mood: string;
}
export interface Order {
  auto_started_at?: string | null;
  server_time?: string;
  step_seconds?: number;
  id: string;
  number: number;
  total: number;
  status: string;
  mood: string;
  note: string;
  quote: string;
  created_at: string;
  items: Line[];
}
export const SESSION_KEY = "mind-offline-session";
export const currentToken = () => uni.getStorageSync(SESSION_KEY) || "";
export class ApiError extends Error {
  constructor(
    message: string,
    public status: number,
  ) {
    super(message);
  }
}
export function request<T>(
  path: string,
  method: "GET" | "POST" = "GET",
  data?: Record<string, unknown>,
): Promise<T> {
  const sentToken = currentToken();
  return new Promise((resolve, reject) =>
    uni.request({
      url: base + "/api" + path,
      method,
      data,
      header: {
        "Content-Type": "application/json",
        Authorization: "Bearer " + sentToken,
      },
      timeout: 15000,
      success: (r) => {
        if (sentToken !== currentToken()) {
          reject(new ApiError("账号已切换，请重试", 0));
          return;
        }
        if (r.statusCode >= 200 && r.statusCode < 300) resolve(r.data as T);
        else
          reject(
            new ApiError(
              (r.data as any)?.error || "食堂暂时忙不过来，请稍后再试",
              r.statusCode,
            ),
          );
      },
      fail: () => reject(new Error("信号也精神离职了，请检查网络后重试")),
    }),
  );
}
