import { useCallback, useEffect, useLayoutEffect, useRef, useState, useSyncExternalStore } from "react";
import { api, errText } from "../api";
import { Icon } from "./ui";

/** One picture the viewer can show. */
export type ViewerImage = {
  key: string;
  /** the full-size picture, when it is already on this Mac */
  src?: string;
  /** fetches the full-size picture when the viewer reaches it */
  load?: () => Promise<string>;
  /** a small preview to show while the full one loads */
  thumb?: string;
  alt: string;
  caption?: string;
  /** the file name Save… suggests */
  name?: string;
  /** shown when there is no full-size picture to load */
  note?: string;
};

type Open = { items: ViewerImage[]; index: number; n: number };

let current: Open | null = null;
let seq = 0;
const listeners = new Set<() => void>();
const publish = (next: Open | null) => {
  current = next;
  listeners.forEach((l) => l());
};

/** openImages shows items in the viewer, starting at index. */
export function openImages(items: ViewerImage[], index = 0) {
  if (items.length === 0) return;
  publish({ items, index: Math.min(Math.max(index, 0), items.length - 1), n: ++seq });
}

export function closeImages() {
  publish(null);
}

const subscribe = (l: () => void) => {
  listeners.add(l);
  return () => listeners.delete(l);
};

/** ImageViewerHost renders the viewer while one is open; mount it once. */
export function ImageViewerHost() {
  const open = useSyncExternalStore(subscribe, () => current);
  if (!open) return null;
  return <ImageViewer key={open.n} items={open.items} start={open.index} onClose={closeImages} />;
}

const MIN = 1;
const MAX = 8;
const clampScale = (s: number) => Math.min(MAX, Math.max(MIN, s));

type View = { s: number; x: number; y: number };
const FIT: View = { s: 1, x: 0, y: 0 };

export function ImageViewer({ items, start, onClose }: { items: ViewerImage[]; start: number; onClose: () => void }) {
  const [index, setIndex] = useState(start);
  const item = items[index];
  // Full-size URLs fetched while the viewer is open. Only the current picture is in the page;
  // closing the viewer drops it, so its decoded pixels can be freed.
  const [loaded, setLoaded] = useState<Record<string, string>>({});
  const [loading, setLoading] = useState(false);
  const [status, setStatus] = useState("");
  const [view, setView] = useState<View>(FIT);
  const stage = useRef<HTMLDivElement>(null);
  const img = useRef<HTMLImageElement | null>(null);
  const dialog = useRef<HTMLDivElement>(null);
  const viewRef = useRef(view);
  viewRef.current = view;

  const full = item.src || loaded[item.key] || "";
  const shown = full || item.thumb || "";

  // Fetch the full picture when the viewer reaches it.
  useEffect(() => {
    if (item.src || loaded[item.key] || !item.load) return;
    let live = true;
    setLoading(true);
    item
      .load()
      .then((u) => live && u && setLoaded((l) => ({ ...l, [item.key]: u })))
      .catch((e) => live && setStatus(errText(e)))
      .finally(() => live && setLoading(false));
    return () => {
      live = false;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [item.key]);

  useEffect(() => {
    setView(FIT);
    setStatus("");
  }, [index]);

  // Keep the focus inside while open and give it back on close.
  useLayoutEffect(() => {
    const before = document.activeElement as HTMLElement | null;
    dialog.current?.focus();
    return () => before?.focus?.();
  }, []);

  // Let WebKit forget a picture as soon as it leaves the viewer (next picture, or closing).
  const imgRef = useCallback((el: HTMLImageElement | null) => {
    if (!el) img.current?.removeAttribute("src");
    img.current = el;
  }, []);

  const step = useCallback((d: number) => setIndex((i) => Math.min(items.length - 1, Math.max(0, i + d))), [items.length]);

  /** clamp keeps the zoomed picture covering the stage instead of drifting off it. */
  const clamp = useCallback((v: View): View => {
    const s = clampScale(v.s);
    const st = stage.current;
    const im = img.current;
    if (s === 1 || !st || !im) return { s, x: 0, y: 0 };
    const mx = Math.max(0, (im.offsetWidth * s - st.clientWidth) / 2);
    const my = Math.max(0, (im.offsetHeight * s - st.clientHeight) / 2);
    return { s, x: Math.min(mx, Math.max(-mx, v.x)), y: Math.min(my, Math.max(-my, v.y)) };
  }, []);

  /** zoomTo scales to s keeping the point (px, py), relative to the stage's centre, still. */
  const zoomTo = useCallback(
    (s: number, px = 0, py = 0) => {
      const v = viewRef.current;
      const next = clampScale(s);
      const k = next / v.s;
      setView(clamp({ s: next, x: px - (px - v.x) * k, y: py - (py - v.y) * k }));
    },
    [clamp],
  );

  const centre = (cx: number, cy: number) => {
    const r = stage.current?.getBoundingClientRect();
    return r ? [cx - r.left - r.width / 2, cy - r.top - r.height / 2] : [0, 0];
  };

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      let handled = true;
      if (e.key === "Escape") onClose();
      else if (e.key === "ArrowLeft" && !e.metaKey) step(-1);
      else if (e.key === "ArrowRight" && !e.metaKey) step(1);
      else if (e.metaKey && (e.key === "=" || e.key === "+")) zoomTo(viewRef.current.s * 1.5);
      else if (e.metaKey && e.key === "-") zoomTo(viewRef.current.s / 1.5);
      else if (e.metaKey && e.key === "0") setView(FIT);
      else handled = false;
      if (handled) {
        e.preventDefault();
        e.stopPropagation();
      }
    };
    window.addEventListener("keydown", onKey, true);
    return () => window.removeEventListener("keydown", onKey, true);
  }, [onClose, step, zoomTo]);

  // Pinch on the trackpad: WebKit sends gesture events, other engines send ctrl+wheel.
  // Two-finger scrolling pans a zoomed picture.
  useEffect(() => {
    const el = stage.current;
    if (!el) return;
    let base = 1;
    const onWheel = (e: WheelEvent) => {
      e.preventDefault();
      const [px, py] = centre(e.clientX, e.clientY);
      if (e.ctrlKey) zoomTo(viewRef.current.s * Math.exp(-e.deltaY * 0.01), px, py);
      else if (viewRef.current.s > 1) {
        const v = viewRef.current;
        setView(clamp({ ...v, x: v.x - e.deltaX, y: v.y - e.deltaY }));
      }
    };
    const onGestureStart = (e: Event) => {
      e.preventDefault();
      base = viewRef.current.s;
    };
    const onGestureChange = (e: Event) => {
      e.preventDefault();
      const g = e as Event & { scale: number; clientX: number; clientY: number };
      const [px, py] = centre(g.clientX, g.clientY);
      zoomTo(base * g.scale, px, py);
    };
    el.addEventListener("wheel", onWheel, { passive: false });
    el.addEventListener("gesturestart", onGestureStart);
    el.addEventListener("gesturechange", onGestureChange);
    return () => {
      el.removeEventListener("wheel", onWheel);
      el.removeEventListener("gesturestart", onGestureStart);
      el.removeEventListener("gesturechange", onGestureChange);
    };
  }, [clamp, zoomTo]);

  // Dragging pans a zoomed picture. A drag never counts as a click outside.
  const drag = useRef<{ x: number; y: number; vx: number; vy: number; moved: boolean } | null>(null);
  const dragged = useRef(false);
  const onPointerDown = (e: React.PointerEvent) => {
    dragged.current = false;
    if (viewRef.current.s <= 1 || e.button !== 0) return;
    drag.current = { x: e.clientX, y: e.clientY, vx: viewRef.current.x, vy: viewRef.current.y, moved: false };
    (e.target as Element).setPointerCapture?.(e.pointerId);
  };
  const onPointerMove = (e: React.PointerEvent) => {
    const d = drag.current;
    if (!d) return;
    const dx = e.clientX - d.x;
    const dy = e.clientY - d.y;
    if (Math.abs(dx) + Math.abs(dy) > 3) d.moved = true;
    setView(clamp({ s: viewRef.current.s, x: d.vx + dx, y: d.vy + dy }));
  };
  const onPointerUp = () => {
    dragged.current = !!drag.current?.moved;
    drag.current = null;
  };

  const act = async (what: "save" | "copy" | "preview") => {
    if (!full) return;
    setStatus("");
    try {
      if (what === "save") {
        const p = await api.saveImage(full, item.name ?? "");
        if (p) setStatus(`Saved to ${p}`);
      } else if (what === "copy") {
        await api.copyImage(full);
        setStatus("Copied");
      } else {
        await api.openImageInPreview(full);
      }
    } catch (e) {
      setStatus(errText(e));
    }
  };

  const zoomed = view.s > 1;
  const many = items.length > 1;
  return (
    <div className="iv" role="dialog" aria-modal="true" aria-label={item.alt} ref={dialog} tabIndex={-1}>
      <div className="iv__bar" role="toolbar" aria-label="Image">
        <span className="iv__count tabular">{many ? `${index + 1} of ${items.length}` : ""}</span>
        <div className="iv__actions">
          <button type="button" className="iv__btn" onClick={() => void act("save")} disabled={!full}>
            Save…
          </button>
          <button type="button" className="iv__btn" onClick={() => void act("copy")} disabled={!full}>
            <Icon name="copy" size={15} />
            Copy
          </button>
          <button type="button" className="iv__btn" onClick={() => void act("preview")} disabled={!full}>
            Open in Preview
          </button>
          <button type="button" className="iv__btn iv__btn--icon" onClick={onClose} aria-label="Close">
            <Icon name="close" size={16} />
          </button>
        </div>
      </div>
      <div
        className={zoomed ? "iv__stage iv__stage--zoomed" : "iv__stage"}
        ref={stage}
        data-testid="iv-stage"
        onClick={(e) => {
          if (e.target === e.currentTarget && !dragged.current) onClose();
        }}
        onPointerDown={onPointerDown}
        onPointerMove={onPointerMove}
        onPointerUp={onPointerUp}
        onPointerCancel={onPointerUp}
      >
        {shown ? (
          <img
            key={item.key}
            ref={imgRef}
            src={shown}
            alt={item.alt}
            className={full ? "iv__img" : "iv__img iv__img--thumb"}
            draggable={false}
            style={{ transform: `translate(${view.x}px, ${view.y}px) scale(${view.s})` }}
            onDoubleClick={(e) => {
              if (zoomed) setView(FIT);
              else {
                const [px, py] = centre(e.clientX, e.clientY);
                zoomTo(2.5, px, py);
              }
            }}
          />
        ) : (
          <span className="iv__empty">{loading ? "" : "Photo not available"}</span>
        )}
        {loading && <span className="spinner iv__spin" aria-label="Loading the full photo" />}
        {many && index > 0 && (
          <button type="button" className="iv__nav iv__nav--prev" onClick={() => step(-1)} aria-label="Previous image">
            <Icon name="chevron" size={20} />
          </button>
        )}
        {many && index < items.length - 1 && (
          <button type="button" className="iv__nav iv__nav--next" onClick={() => step(1)} aria-label="Next image">
            <Icon name="chevron" size={20} />
          </button>
        )}
      </div>
      {(item.caption || status || (!full && item.note)) && (
        <div className="iv__foot">
          {item.caption && <span className="iv__caption">{item.caption}</span>}
          {!full && item.note && <span className="iv__note">{item.note}</span>}
          {status && (
            <span className="iv__status" role="status">
              {status}
            </span>
          )}
        </div>
      )}
    </div>
  );
}
