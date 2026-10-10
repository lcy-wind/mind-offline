import { currentToken, ApiError, apiBase } from "./api";
import { createChatEventParser, type ChatEvent } from "./healing-stream";

// WeChat does not consistently provide TextDecoder. Retain an incomplete UTF-8
// code point across chunks, including four-byte emoji.
export function createUTF8Decoder() {
  let pending = new Uint8Array(0);
  return (chunk = new ArrayBuffer(0), final = false) => {
    const bytes = new Uint8Array(pending.length + chunk.byteLength);
    bytes.set(pending); bytes.set(new Uint8Array(chunk), pending.length);
    let end = bytes.length;
    if (!final && end) {
      let start = end - 1;
      while (start > 0 && (bytes[start] & 0xc0) === 0x80) start--;
      const first = bytes[start];
      const length = first < 0x80 ? 1 : first < 0xe0 ? 2 : first < 0xf0 ? 3 : 4;
      if (end - start < length) end = start;
    }
    pending = bytes.slice(end);
    let encoded = "";
    for (let i = 0; i < end; i++) encoded += "%" + bytes[i].toString(16).padStart(2, "0");
    return decodeURIComponent(encoded);
  };
}
export function nativeHealing(id: string, requestID: string, text: string, accept: (event: ChatEvent) => void) {
  const token = currentToken();
  let task: UniApp.RequestTask & { onChunkReceived?: (callback: (result: {data: ArrayBuffer}) => void) => void };
  let settled = false, finished = false, received = false, status = 0;
  let rejectRequest: (error: Error) => void = () => {};
  const promise = new Promise<void>((resolve, reject) => {
    rejectRequest = reject;
    const fail = (error: Error) => { if (!settled) { settled = true; reject(error); task?.abort(); } };
    const decode = createUTF8Decoder();
    const checkSession = () => { if (currentToken() !== token) throw new ApiError("账号已切换", 0); };
    const parse = createChatEventParser(event => {
      checkSession();
      if (event.event === "error") throw new Error(event.data.message || "回复没有完成，请重试");
      if (event.event === "done") finished = true;
      accept(event);
    });
    task = uni.request({
      url: apiBase() + "/api/healing/conversations/" + encodeURIComponent(id) + "/messages",
      method: "POST", data: {request_id: requestID, text},
      header: {"Content-Type": "application/json", Authorization: "Bearer " + token},
      enableChunked: true, responseType: "text", dataType: "text", timeout: 100000,
      success: response => {
        if (settled) return;
        try {
          checkSession();
          if (response.statusCode < 200 || response.statusCode >= 300) {
            let data: any = response.data;
            if (typeof data === "string") { try { data = JSON.parse(data); } catch { data = {}; } }
            throw new ApiError(data?.error || "聊天暂时没有连上，请重试", response.statusCode);
          }
          // Some clients buffer the response instead of delivering chunks.
          if (!received && typeof response.data === "string") parse(response.data);
          else parse(decode(undefined, true));
          if (!finished) throw new Error("连接中断，回复未完成，请刷新或重试");
          settled = true; resolve();
        } catch (e) { fail(e as Error); }
      },
      fail: error => fail(new Error(error.errMsg.includes("timeout") ? "回复等待较久，请稍后重试" : "聊天连接中断，请重试")),
    });
    task.onHeadersReceived(result => { status = result.statusCode || 0; });
    task.onChunkReceived?.(result => {
      if (settled || (status && (status < 200 || status >= 300))) return;
      try { checkSession(); received = true; parse(decode(result.data)); }
      catch (e) { fail(e as Error); }
    });
  });
  return { promise, abort: () => {
    if (settled) return;
    settled = true;
    const error = new Error("Stopped"); error.name = "AbortError";
    rejectRequest(error); task?.abort();
  }};
}
