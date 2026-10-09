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
export function request<T>(
  path: string,
  method: "GET" | "POST" = "GET",
  data?: Record<string, unknown>,
): Promise<T> {
  return new Promise((resolve, reject) =>
    uni.request({
      url: base + "/api" + path,
      method,
      data,
      header: {
        "Content-Type": "application/json",
        Authorization:
          "Bearer " + (uni.getStorageSync("mind-offline-token") || ""),
      },
      timeout: 15000,
      success: (r) => {
        if (r.statusCode >= 200 && r.statusCode < 300) resolve(r.data as T);
        else
          reject(
            new Error((r.data as any)?.error || "食堂暂时忙不过来，请稍后再试"),
          );
      },
      fail: () => reject(new Error("信号也精神离职了，请检查网络后重试")),
    }),
  );
}
export async function connect() {
  if (!uni.getStorageSync("mind-offline-token")) {
    const s = await request<{ token: string }>("/guest", "POST");
    uni.setStorageSync("mind-offline-token", s.token);
  }
  return request<Guest>("/me");
}
