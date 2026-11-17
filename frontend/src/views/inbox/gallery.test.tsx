import { act, fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import type { ChatMessage, Media } from "../../api";

const getMedia = vi.fn(async () => "/media/p2.jpg");
const openMedia = vi.fn(async () => {});
vi.mock("../../api", async (orig) => {
  const real = await orig<typeof import("../../api")>();
  return { ...real, inbox: { ...real.inbox, getMedia, openMedia } };
});

const { chatGallery, imageName, viewerImage, ONLY_ON_PHONE } = await import("./gallery");
const { MessageBubble } = await import("./MessageBubble");
const { ImageViewerHost, closeImages } = await import("../../components/ImageViewer");

const media = (p: Partial<Media> = {}): Media => ({ mime: "image/jpeg", seconds: 0, fileName: "", size: 0, url: "", thumbUrl: "", downloadable: true, ...p });
const msg = (p: Partial<ChatMessage>): ChatMessage =>
  ({
    id: "m",
    chatJid: "ana@s.whatsapp.net",
    senderJid: "ana@s.whatsapp.net",
    senderName: "Ana",
    fromMe: false,
    fromAgent: false,
    agentName: "",
    fromUi: false,
    at: new Date(2026, 10, 16, 14, 3, 27).getTime(),
    kind: "image",
    text: "",
    transcript: "",
    mentions: [],
    mentionsMe: false,
    quoted: null,
    reactions: [],
    media: media(),
    status: "",
    edited: false,
    revoked: false,
    history: false,
    viewOnce: false,
    ...p,
  }) as ChatMessage;

describe("chat gallery", () => {
  const thread = [
    msg({ id: "p1", media: media({ url: "/media/p1.jpg" }) }),
    msg({ id: "t1", kind: "text", media: null, text: "hi" }),
    msg({ id: "v1", kind: "video", media: media({ mime: "video/mp4" }) }),
    msg({ id: "once", viewOnce: true }),
    msg({ id: "gone", revoked: true }),
    msg({ id: "old", media: media({ downloadable: false }) }), // nothing to show at all
    msg({ id: "s1", kind: "sticker", media: media({ url: "/media/s1.webp" }) }),
    msg({ id: "p2", text: "beach", media: media({ thumbUrl: "/media/p2.thumb.jpg" }) }),
    msg({ id: "p3", media: media({ downloadable: false, thumbUrl: "/media/p3.thumb.jpg" }) }),
  ];

  it("steps through the chat's photos in order, starting at the one clicked", () => {
    const g = chatGallery(thread, thread[7]);
    expect(g.items.map((i) => i.key)).toEqual(["p1", "p2", "p3"]);
    expect(g.index).toBe(1);
    expect(g.items[1]).toMatchObject({ src: undefined, thumb: "/media/p2.thumb.jpg", alt: "Photo: beach", caption: "beach" });
    expect(g.items[2].note).toBe(ONLY_ON_PHONE);
    expect(g.items[2].load).toBeUndefined();
  });

  it("opens a sticker on its own", () => {
    const g = chatGallery(thread, thread[6]);
    expect(g.items.map((i) => i.key)).toEqual(["s1"]);
    expect(g.items[0].alt).toBe("Sticker");
  });

  it("fetches a photo that is not downloaded yet only when asked", async () => {
    const it = viewerImage(thread[7]);
    expect(getMedia).not.toHaveBeenCalled();
    expect(await it.load!()).toBe("/media/p2.jpg");
    expect(getMedia).toHaveBeenCalledWith("ana@s.whatsapp.net", "p2");
  });

  it("suggests a file name like a macOS screenshot", () => {
    expect(imageName(thread[0])).toBe("WhatsApp photo 2026-11-16 at 14.03.27");
    expect(imageName(thread[6])).toBe("WhatsApp sticker 2026-11-16 at 14.03.27");
  });

  it("a photo bubble opens the viewer, not Preview", () => {
    const actions = { onReply: () => {}, onEdit: () => {}, onJump: () => {}, onError: () => {} };
    const onOpenImage = vi.fn();
    const p1 = thread[0];
    const { unmount } = render(<MessageBubble m={p1} isGroup={false} continued={false} highlight={false} actions={{ ...actions, onOpenImage }} />);
    fireEvent.click(screen.getByRole("button", { name: "Photo" }));
    expect(onOpenImage).toHaveBeenCalledWith(p1);
    expect(openMedia).not.toHaveBeenCalled();
    unmount();

    // Without a chat around it, the bubble opens the photo on its own.
    render(
      <>
        <MessageBubble m={p1} isGroup={false} continued={false} highlight={false} actions={actions} />
        <ImageViewerHost />
      </>,
    );
    fireEvent.click(screen.getByRole("button", { name: "Photo" }));
    expect(screen.getByRole("dialog", { name: "Photo" }).querySelector("img")).toHaveAttribute("src", "/media/p1.jpg");
    expect(openMedia).not.toHaveBeenCalled();
    act(() => closeImages());
  });
});
