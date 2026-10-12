// Apply to assistant messages only, including existing saved replies.
export function formatHealingReply(text: string): string {
  return text.replace(/\r\n?/g, "\n").replace(/[ \t]*\n(?:[ \t]*\n)+/g, "\n").trim();
}
