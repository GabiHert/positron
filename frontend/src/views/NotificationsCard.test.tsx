import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import type { InboxChat, Settings } from "../api";

const setNotifications = vi.fn(async () => {});
const openNotificationSettings = vi.fn(async () => {});
vi.mock("../api", async (orig) => {
  const real = await orig<typeof import("../api")>();
  return { ...real, api: { ...real.api, openNotificationSettings }, inbox: { ...real.inbox, setNotifications } };
});

const { NotificationsCard } = await import("./SettingsView");
const { NotifyToggle } = await import("./inbox/Conversation");

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

  it("the bell mutes a chat in Positron and back", async () => {
    const chat = { jid: "ana@s.whatsapp.net", name: "Ana", notifyOff: false, muted: false } as InboxChat;
    const onError = vi.fn();
    render(<NotifyToggle chat={chat} onError={onError} />);
    fireEvent.click(screen.getByRole("button", { name: "Turn off notifications for this chat in Positron" }));
    expect(setNotifications).toHaveBeenLastCalledWith("ana@s.whatsapp.net", false);
    const on = await screen.findByRole("button", { name: /Notifications off for this chat/ });
    expect(on).toHaveAttribute("aria-pressed", "true");
    setNotifications.mockRejectedValueOnce(new Error("no"));
    fireEvent.click(on);
    expect(setNotifications).toHaveBeenLastCalledWith("ana@s.whatsapp.net", true);
    await screen.findByRole("button", { name: /Notifications off for this chat/ }); // rolled back
    expect(onError).toHaveBeenCalledWith("no");
  });
});
