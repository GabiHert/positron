import { act, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

const saveImage = vi.fn(async () => "/Users/me/Desktop/a.jpg");
const copyImage = vi.fn(async () => {});
const openImageInPreview = vi.fn(async () => {});
vi.mock("../api", async (orig) => {
  const real = await orig<typeof import("../api")>();
  return { ...real, api: { ...real.api, saveImage, copyImage, openImageInPreview } };
});

const { ImageViewerHost, openImages, closeImages } = await import("./ImageViewer");

const key = (k: string, meta = false) => fireEvent.keyDown(window, { key: k, metaKey: meta });
const shownImg = () => screen.getByRole("dialog").querySelector("img")!;

describe("ImageViewer", () => {
  afterEach(() => {
    act(() => closeImages());
    vi.clearAllMocks();
  });

  it("opens on the clicked image, steps with the arrows and closes with Esc", () => {
    render(<ImageViewerHost />);
    expect(screen.queryByRole("dialog")).toBeNull();
    act(() =>
      openImages(
        [
          { key: "a", src: "/media/a.jpg", alt: "Photo: one" },
          { key: "b", src: "/media/b.jpg", alt: "Photo: two", caption: "two" },
          { key: "c", src: "/media/c.jpg", alt: "Photo: three" },
        ],
        1,
      ),
    );
    expect(screen.getByRole("dialog", { name: "Photo: two" })).toBeInTheDocument();
    expect(screen.getByText("2 of 3")).toBeInTheDocument();
    expect(screen.getByText("two")).toBeInTheDocument();
    key("ArrowRight");
    expect(shownImg()).toHaveAttribute("src", "/media/c.jpg");
    key("ArrowRight"); // stays on the last one
    expect(screen.getByText("3 of 3")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Next image" })).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "Previous image" }));
    key("ArrowLeft");
    expect(shownImg()).toHaveAttribute("src", "/media/a.jpg");
    key("Escape");
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it("closes on a click outside the picture, not on the picture", () => {
    render(<ImageViewerHost />);
    act(() => openImages([{ key: "a", src: "/media/a.jpg", alt: "Photo" }]));
    fireEvent.click(shownImg());
    expect(screen.getByRole("dialog")).toBeInTheDocument();
    expect(screen.queryByText(/of 1/)).toBeNull();
    fireEvent.click(screen.getByTestId("iv-stage"));
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it("loads the full picture only when reached and lets it go on close", async () => {
    let resolve: (u: string) => void = () => {};
    const load = vi.fn(() => new Promise<string>((r) => (resolve = r)));
    render(<ImageViewerHost />);
    act(() =>
      openImages([
        { key: "a", src: "/media/a.jpg", alt: "first" },
        { key: "b", thumb: "/media/b.thumb.jpg", load, alt: "second" },
      ]),
    );
    expect(load).not.toHaveBeenCalled();
    key("ArrowRight");
    expect(load).toHaveBeenCalledTimes(1);
    expect(shownImg()).toHaveAttribute("src", "/media/b.thumb.jpg");
    expect(screen.getByRole("button", { name: "Save…" })).toBeDisabled();
    expect(screen.getByLabelText("Loading the full photo")).toBeInTheDocument();
    await act(async () => resolve("/media/b.jpg"));
    const img = shownImg();
    expect(img).toHaveAttribute("src", "/media/b.jpg");
    expect(screen.getByRole("button", { name: "Save…" })).toBeEnabled();
    key("ArrowLeft");
    key("ArrowRight");
    expect(load).toHaveBeenCalledTimes(1); // kept while the viewer is open
    const again = shownImg();
    act(() => closeImages());
    expect(again).not.toHaveAttribute("src");
  });

  it("says when the full photo is not on this Mac", () => {
    render(<ImageViewerHost />);
    act(() => openImages([{ key: "a", thumb: "/media/a.thumb.jpg", alt: "Photo", note: "The full photo is only on your phone." }]));
    expect(screen.getByText("The full photo is only on your phone.")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Copy" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "Open in Preview" })).toBeDisabled();
  });

  it("zooms with ⌘+ and ⌘−, double-click, and resets with ⌘0", () => {
    render(<ImageViewerHost />);
    act(() => openImages([{ key: "a", src: "/media/a.jpg", alt: "Photo" }]));
    const scale = () => /scale\(([\d.]+)\)/.exec(shownImg().style.transform)?.[1];
    expect(scale()).toBe("1");
    key("=", true);
    expect(scale()).toBe("1.5");
    key("-", true);
    expect(scale()).toBe("1");
    key("-", true); // never smaller than the fitted size
    expect(scale()).toBe("1");
    fireEvent.doubleClick(shownImg());
    expect(scale()).toBe("2.5");
    key("0", true);
    expect(scale()).toBe("1");
    fireEvent.wheel(screen.getByTestId("iv-stage"), { ctrlKey: true, deltaY: -50 });
    expect(Number(scale())).toBeGreaterThan(1);
    fireEvent.doubleClick(shownImg());
    expect(scale()).toBe("1");
  });

  it("saves, copies and opens in Preview the full picture", async () => {
    render(<ImageViewerHost />);
    act(() => openImages([{ key: "a", src: "/media/a.jpg", alt: "Photo", name: "WhatsApp photo" }]));
    await act(async () => fireEvent.click(screen.getByRole("button", { name: "Save…" })));
    expect(saveImage).toHaveBeenCalledWith("/media/a.jpg", "WhatsApp photo");
    expect(screen.getByRole("status")).toHaveTextContent("Saved to /Users/me/Desktop/a.jpg");
    await act(async () => fireEvent.click(screen.getByRole("button", { name: "Copy" })));
    expect(copyImage).toHaveBeenCalledWith("/media/a.jpg");
    expect(screen.getByRole("status")).toHaveTextContent("Copied");
    await act(async () => fireEvent.click(screen.getByRole("button", { name: "Open in Preview" })));
    expect(openImageInPreview).toHaveBeenCalledWith("/media/a.jpg");
    copyImage.mockRejectedValueOnce(new Error("macOS could not read this image"));
    await act(async () => fireEvent.click(screen.getByRole("button", { name: "Copy" })));
    expect(screen.getByRole("status")).toHaveTextContent("macOS could not read this image");
  });
});
