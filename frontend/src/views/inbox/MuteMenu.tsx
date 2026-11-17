import { useEffect, useRef, useState } from "react";
import { errText, inbox, type InboxChat, type MuteLength } from "../../api";
import { Icon } from "../../components/ui";
import { isMuted, muteLabel } from "./helpers";

const choices: { length: MuteLength; label: string }[] = [
  { length: "8h", label: "8 hours" },
  { length: "1w", label: "1 week" },
  { length: "always", label: "Always" },
];

/** MuteMenu is the conversation header's mute button: mute for 8 hours, 1 week or always, or
 * unmute, the same as WhatsApp's and synced with the phone. */
export function MuteMenu({ chat, onError }: { chat: InboxChat; onError: (t: string) => void }) {
  const [open, setOpen] = useState(false);
  const [busy, setBusy] = useState(false);
  // What we set, shown until the chat list catches up.
  const [local, setLocal] = useState<Pick<InboxChat, "mutedUntil" | "notifyOff"> | null>(null);
  const btn = useRef<HTMLButtonElement>(null);
  const menu = useRef<HTMLDivElement>(null);
  useEffect(() => setLocal(null), [chat.jid, chat.mutedUntil, chat.notifyOff]);

  const state = local ?? chat;
  const muted = isMuted(state);
  const label = muteLabel(state);

  useEffect(() => {
    if (!open) return;
    const onDown = (e: MouseEvent) => {
      if (!menu.current?.contains(e.target as Node) && !btn.current?.contains(e.target as Node)) setOpen(false);
    };
    document.addEventListener("mousedown", onDown);
    menu.current?.querySelector<HTMLElement>("button")?.focus();
    return () => document.removeEventListener("mousedown", onDown);
  }, [open]);

  const onMenuKey = (e: React.KeyboardEvent) => {
    const items = Array.from(menu.current?.querySelectorAll<HTMLElement>('[role="menuitem"]') ?? []);
    const i = items.indexOf(document.activeElement as HTMLElement);
    if (e.key === "ArrowDown") {
      e.preventDefault();
      items[(i + 1) % items.length]?.focus();
    } else if (e.key === "ArrowUp") {
      e.preventDefault();
      items[(i - 1 + items.length) % items.length]?.focus();
    } else if (e.key === "Escape" || e.key === "Tab") {
      setOpen(false);
      btn.current?.focus();
    }
  };

  const pick = async (length: MuteLength) => {
    setOpen(false);
    btn.current?.focus();
    setBusy(true);
    try {
      const until = await inbox.mute(chat.jid, length);
      setLocal({ mutedUntil: until, notifyOff: length === "off" ? false : chat.notifyOff });
    } catch (e) {
      onError(`Couldn't change the mute on WhatsApp: ${errText(e)}`);
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="ib-mute">
      <button
        ref={btn}
        type="button"
        className={muted ? "btn btn--sm ib-bell ib-bell--off" : "btn btn--sm ib-bell"}
        aria-haspopup="menu"
        aria-expanded={open}
        aria-label={muted ? `${label}. Change or unmute` : "Mute notifications"}
        title={muted ? label : "Mute notifications"}
        disabled={busy}
        onClick={() => setOpen((v) => !v)}
      >
        <Icon name={muted ? "muted" : "bell"} size={15} />
      </button>
      {open && (
        <div ref={menu} className="ib-menu" role="menu" aria-label="Mute notifications" onKeyDown={onMenuKey}>
          <p className="ib-menu__title">
            {muted ? label : "Mute notifications for"}
            <span className="ib-menu__help">Synced with WhatsApp on your phone. The chat's agent keeps answering.</span>
          </p>
          {choices.map((c) => (
            <button key={c.length} type="button" role="menuitem" className="ib-menu__item" onClick={() => pick(c.length)}>
              <span className="ib-menu__check" />
              <span>{c.label}</span>
            </button>
          ))}
          {muted && (
            <button type="button" role="menuitem" className="ib-menu__item" onClick={() => pick("off")}>
              <span className="ib-menu__check">
                <Icon name="bell" size={14} />
              </span>
              <span>Unmute</span>
            </button>
          )}
        </div>
      )}
    </div>
  );
}
