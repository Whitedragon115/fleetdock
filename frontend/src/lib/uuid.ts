/**
 * randomUUID returns a random RFC 4122 version-4 UUID.
 *
 * crypto.randomUUID() exists only in secure contexts (HTTPS or localhost), so
 * a dashboard opened over plain HTTP on a LAN address (http://192.168.x.y)
 * does not have it. crypto.getRandomValues() is available in every context,
 * so the UUID is assembled from it when randomUUID is missing.
 */
export function randomUUID(): string {
  if (typeof crypto.randomUUID === "function") return crypto.randomUUID();
  const b = crypto.getRandomValues(new Uint8Array(16));
  b[6] = (b[6] & 0x0f) | 0x40; // version 4
  b[8] = (b[8] & 0x3f) | 0x80; // RFC 4122 variant
  const hex = Array.from(b, (x) => x.toString(16).padStart(2, "0")).join("");
  return `${hex.slice(0, 8)}-${hex.slice(8, 12)}-${hex.slice(12, 16)}-${hex.slice(16, 20)}-${hex.slice(20)}`;
}
