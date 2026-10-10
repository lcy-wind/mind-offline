export interface LyricLine { time: number; text: string; translation?: string }
export interface Lyrics { lines: LyricLine[]; plain: string[] }
function readLRC(input: string): Lyrics {
  const lines: LyricLine[] = [], plain: string[] = [];
  const offset = Number(input.match(/\[offset\s*:\s*([+-]?\d+)\]/i)?.[1] || 0) / 1000;
  for (const raw of input.replace(/^\uFEFF/, "").split(/\r?\n/).slice(0, 2000)) {
    const stamps = [...raw.matchAll(/\[(\d{1,3}):(\d{2})(?:\.(\d{1,3}))?\]/g)];
    const text = raw.replace(/\[\d{1,3}:\d{2}(?:\.\d{1,3})?\]/g, "").trim();
    if (!text || /^\[(?:ar|ti|al|by|offset|length|re|ve):.*\]$/i.test(text)) continue;
    if (!stamps.length) { plain.push(text); continue; }
    for (const match of stamps) {
      if (Number(match[2]) >= 60) continue;
      const time = Number(match[1]) * 60 + Number(match[2]) + Number("0." + (match[3] || "0")) - offset;
      lines.push({ time: Math.max(0, time), text });
    }
  }
  lines.sort((a, b) => a.time - b.time);
  // Two vocal lines can share a timestamp. Keep both without duplicate rows.
  const merged: LyricLine[] = [];
  for (const line of lines) {
    const previous = merged[merged.length - 1];
    if (previous && previous.time === line.time) {
      if (!previous.text.split(" / ").includes(line.text)) previous.text += " / " + line.text;
    } else merged.push({ ...line });
  }
  return { lines: merged, plain };
}
export function parseLyrics(raw: string, translation = ""): Lyrics {
  const result = readLRC(raw);
  const translated = readLRC(translation);
  let index = 0;
  for (const line of result.lines) {
    while (index < translated.lines.length && translated.lines[index].time < line.time - 0.15) index++;
    const match = translated.lines[index];
    if (match && Math.abs(match.time - line.time) <= 0.15 && match.text !== line.text) line.translation = match.text;
  }
  if (!result.lines.length && !result.plain.length) return translated;
  return result;
}
export function activeLyricIndex(lines: LyricLine[], seconds: number): number {
  if (!Number.isFinite(seconds)) return -1;
  let left = 0, right = lines.length - 1, result = -1;
  while (left <= right) {
    const middle = (left + right) >>> 1;
    if (lines[middle].time <= seconds) { result = middle; left = middle + 1; }
    else right = middle - 1;
  }
  return result;
}
