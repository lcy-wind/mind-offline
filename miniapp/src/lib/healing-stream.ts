import { currentToken, ApiError } from "./api";
export interface ChatEvent { event: string; data: any }
export function createChatEventParser(accept: (event: ChatEvent) => void) {
  let buffer = "";
  return (chunk: string) => {
    buffer += chunk;
    if (buffer.length > 256000) throw new Error("回复数据过长");
    let separator: RegExpExecArray | null;
    while ((separator = /\r?\n\r?\n/.exec(buffer))) {
      const block = buffer.slice(0, separator.index);
      buffer = buffer.slice(separator.index + separator[0].length);
      let event = "message";
      const data: string[] = [];
      for (const line of block.split(/\r?\n/)) {
        if (line.startsWith("event:")) event = line.slice(6).trim();
        if (line.startsWith("data:")) data.push(line.slice(5).trimStart());
      }
      if (data.length) accept({ event, data: JSON.parse(data.join("\n")) });
    }
  };
}
export async function streamHealing(id: string, requestID: string, text: string, signal: AbortSignal, accept: (event: ChatEvent) => void) {
  const token = currentToken();
  const response = await fetch("/api/healing/conversations/" + encodeURIComponent(id) + "/messages", {
    method: "POST", headers: {"Content-Type":"application/json", Authorization:"Bearer " + token},
    body: JSON.stringify({request_id: requestID, text}), signal,
  });
  if (token !== currentToken()) throw new ApiError("账号已切换", 0);
  if (!response.ok) {
    const data = await response.json().catch(() => ({}));
    throw new ApiError(data.error || "聊天暂时没有连上，请重试", response.status);
  }
  if (!response.body) throw new Error("浏览器暂不支持流式聊天");
  let finished = false;
  const parse = createChatEventParser(e => {
    if (token !== currentToken() || signal.aborted) throw new DOMException("Stopped", "AbortError");
    if (e.event === "error") throw new Error(e.data.message || "回复没有完成，请重试");
    if (e.event === "done") finished = true;
    accept(e);
  });
  const reader = response.body.getReader();
  const decoder = new TextDecoder();
  try {
    while (true) {
      const {done, value} = await reader.read();
      if (token !== currentToken() || signal.aborted) throw new DOMException("Stopped", "AbortError");
      if (done) break;
      parse(decoder.decode(value, {stream:true}));
    }
    parse(decoder.decode());
    if (!finished) throw new Error("连接中断，回复未完成，请刷新或重试");
  } finally { await reader.cancel().catch(() => {}); reader.releaseLock(); }
}
