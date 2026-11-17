import { act, fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import type { InboxChat, Settings } from "../api";

const mute = vi.fn(async (_chat: string, length: string) => (length === "off" ? 0 : Date.now() + 8 * 3600_000));
const openNotificationSettings = vi.fn(async () => {});
vi.mock("../api", async (orig) => {
  const real = await orig<typeof import("../api")>();
  return { ...real, api: { ...real.api, openNotificationSettings }, inbox: { ...real.inbox, mute } };
});

const { NotificationsCard } = await import("./SettingsView");
const { MuteMenu } = await import("./inbox/MuteMenu");

const settings = {
  notify: true,
  notifyMessages: true,
  notifyPreview: true,
  notifySound: true,
  notifyGroups: "all",
} as Settings;

describe("notifications", () => {
  it("changes each setting, and greys out the message options when messages are off", () => {
    const update = vi.fn();
    const { rerender } = render(<NotificationsCard settings={settings} update={update} />);
    fireEvent.click(screen.getByLabelText(/Show a preview/));
    expect(update).toHaveBeenLastCalledWith({ ...settings, notifyPreview: false });
    fireEvent.change(screen.getByLabelText("Group chats"), { target: { value: "mentions" } });
    expect(update).toHaveBeenLastCalledWith({ ...settings, notifyGroups: "mentions" });
    fireEvent.click(screen.getByLabelText(/Play a sound/));
    expect(update).toHaveBeenLastCalledWith({ ...settings, notifySound: false });
    fireEvent.click(screen.getByLabelText(/When an agent needs me/));
    expect(update).toHaveBeenLastCalledWith({ ...settings, notify: false });

    rerender(<NotificationsCard settings={{ ...settings, notifyMessages: false }} update={update} />);
    expect(screen.getByLabelText(/Show a preview/)).toBeDisabled();
    expect(screen.getByLabelText("Group chats")).toBeDisabled();
    expect(screen.getByLabelText(/When an agent needs me/)).toBeEnabled();
  });

  it("says when macOS has notifications off and opens its settings", () => {
    const { rerender } = render(<NotificationsCard settings={settings} update={() => {}} />);
    expect(screen.queryByRole("alert")).toBeNull();
    rerender(<NotificationsCard settings={{ ...settings, notifyBlocked: true }} update={() => {}} />);
    expect(screen.getByRole("alert")).toHaveTextContent("macOS has notifications turned off for Positron");
    fireEvent.click(screen.getByRole("button", { name: "Open Notification Settings" }));
    expect(openNotificationSettings).toHaveBeenCalled();
  });

  it("mutes a chat on WhatsApp from the header and unmutes it", async () => {
    const chat = { jid: "ana@s.whatsapp.net", name: "Ana", notifyOff: false, muted: false, mutedUntil: 0 } as InboxChat;
    const onError = vi.fn();
    render(<MuteMenu chat={chat} onError={onError} />);
    fireEvent.click(screen.getByRole("button", { name: "Mute notifications" }));
    expect(screen.getByText(/Synced with WhatsApp/)).toBeInTheDocument();
    expect(screen.queryByRole("menuitem", { name: "Unmute" })).toBeNull();
    await act(async () => fireEvent.click(screen.getByRole("menuitem", { name: "8 hours" })));
    expect(mute).toHaveBeenLastCalledWith("ana@s.whatsapp.net", "8h");
    const muted = screen.getByRole("button", { name: /^Muted until .*Change or unmute$/ });

    fireEvent.click(muted);
    await act(async () => fireEvent.click(screen.getByRole("menuitem", { name: "Unmute" })));
    expect(mute).toHaveBeenLastCalledWith("ana@s.whatsapp.net", "off");
    expect(screen.getByRole("button", { name: "Mute notifications" })).toBeInTheDocument();

    mute.mockRejectedValueOnce(new Error("not connected"));
    fireEvent.click(screen.getByRole("button", { name: "Mute notifications" }));
    await act(async () => fireEvent.click(screen.getByRole("menuitem", { name: "Always" })));
    expect(onError).toHaveBeenCalledWith("Couldn't change the mute on WhatsApp: not connected");
    expect(screen.getByRole("button", { name: "Mute notifications" })).toBeInTheDocument();
  });

  it("shows a chat muted always, or in Positron only, as muted", () => {
    const always = { jid: "g@g.us", name: "Família", notifyOff: false, mutedUntil: new Date(9999, 0, 1).getTime() } as InboxChat;
    const { rerender } = render(<MuteMenu chat={always} onError={() => {}} />);
    expect(screen.getByRole("button", { name: "Muted always. Change or unmute" })).toBeInTheDocument();
    rerender(<MuteMenu chat={{ ...always, jid: "b", mutedUntil: 0, notifyOff: true }} onError={() => {}} />);
    expect(screen.getByRole("button", { name: "Muted in Positron only. Change or unmute" })).toBeInTheDocument();
  });
});
