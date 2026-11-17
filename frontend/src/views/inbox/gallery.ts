import { inbox, type ChatMessage } from "../../api";
import type { ViewerImage } from "../../components/ImageViewer";

export const ONLY_ON_PHONE = "The full photo is only on your phone.";

/** inGallery says whether a message is a photo the viewer can step to. */
export function inGallery(m: ChatMessage): boolean {
  const md = m.media;
  return m.kind === "image" && !m.viewOnce && !m.revoked && !!md && !!(md.url || md.downloadable || md.thumbUrl);
}

const pad = (n: number) => String(n).padStart(2, "0");

/** imageName is what Save… suggests, in the style of a macOS screenshot: "WhatsApp photo 2026-11-16 at 14.03.27". */
export function imageName(m: Pick<ChatMessage, "kind" | "at">): string {
  const d = new Date(m.at);
  const day = `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`;
  const time = `${pad(d.getHours())}.${pad(d.getMinutes())}.${pad(d.getSeconds())}`;
  return `WhatsApp ${m.kind === "sticker" ? "sticker" : "photo"} ${day} at ${time}`;
}

/** viewerImage turns a photo or sticker message into a viewer item that fetches the full file only when shown. */
export function viewerImage(m: ChatMessage): ViewerImage {
  const md = m.media;
  const url = md?.url || "";
  return {
    key: m.id,
    src: url || undefined,
    thumb: md?.thumbUrl || undefined,
    load: !url && md?.downloadable ? () => inbox.getMedia(m.chatJid, m.id) : undefined,
    alt: m.kind === "sticker" ? "Sticker" : m.text ? `Photo: ${m.text}` : "Photo",
    caption: m.text || undefined,
    name: imageName(m),
    note: !url && !md?.downloadable ? ONLY_ON_PHONE : undefined,
  };
}

/**
 * chatGallery is what the viewer steps through when m is opened: the chat's photos in the order
 * they were sent, starting at m. A sticker opens on its own.
 */
export function chatGallery(messages: ChatMessage[], m: ChatMessage): { items: ViewerImage[]; index: number } {
  if (m.kind !== "image") return { items: [viewerImage(m)], index: 0 };
  const photos = messages.filter((x) => x.id === m.id || inGallery(x));
  const index = Math.max(0, photos.findIndex((x) => x.id === m.id));
  return { items: photos.map(viewerImage), index };
}
