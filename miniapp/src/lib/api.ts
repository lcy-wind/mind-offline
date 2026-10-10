const HOST = "https://mind-offline.duckdns.org";
let base = HOST;
// #ifdef H5
base = "";
// #endif
export const apiBase = () => base;
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
export function networkFailureMessage(errMsg = "") {
  const detail = errMsg.replace(/https?:\/\/[^\s"'<>]+/g, url => url.replace(/(https?:\/\/[^/?#]+).*/, "$1"))
    .replace(/Bearer\s+[^\s]+/gi, "Bearer [已隐藏]").replace(/[\r\n]+/g, " ").slice(0, 220);
  let code = "NETWORK", hint = "未能连接食堂服务器，请换用手机流量或稍后重试。";
  if (/domain list|url.*合法|域名.*合法/i.test(errMsg)) { code = "DOMAIN"; hint = "微信未允许访问食堂服务器，请联系管理员更新服务器域名配置。"; }
  else if (/timeout|timed out/i.test(errMsg)) { code = "TIMEOUT"; hint = "连接食堂服务器超时，请换个网络重试。"; }
  else if (/ssl|tls|certificate|cert_/i.test(errMsg)) { code = "TLS"; hint = "与食堂服务器的安全连接失败，请确认手机时间正确并联系管理员。"; }
  else if (/dns|resolve|name_not_resolved/i.test(errMsg)) { code = "DNS"; hint = "手机暂时无法解析食堂域名，请换个网络重试。"; }
  return `连接失败（NET-${code}）。${hint}${detail ? "\n网络提示：" + detail : ""}`;
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
      fail: (result) => reject(new Error(networkFailureMessage(result.errMsg))),
    }),
  );
}
