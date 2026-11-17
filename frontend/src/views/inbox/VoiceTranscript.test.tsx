import { act, fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import type { ChatMessage, Settings } from "../../api";

let finish: (text: string) => void = () => {};
const transcribe = vi.fn(() => new Promise<string>((resolve) => (finish = resolve)));
vi.mock("../../api", async (orig) => {
  const real = await orig<typeof import("../../api")>();
  return { ...real, inbox: { ...real.inbox, transcribe } };
});
const { VoiceTranscript } = await import("./VoiceTranscript");
const { AutoTranscribeField } = await import("../SettingsView");

const voice = { id: "v1", chatJid: "ana@s.whatsapp.net", kind: "voice", transcript: "", transcribing: false, revoked: false, viewOnce: false } as ChatMessage;
const long = "Oi Gabriel, tudo bem? Passando pra lembrar da reunião de amanhã às sete, leva os papéis do apartamento e o contrato assinado, por favor.";

describe("voice transcripts", () => {
  it("transcribes on click, showing Transcribing… meanwhile", async () => {
    render(<VoiceTranscript m={voice} onError={() => {}} />);
    fireEvent.click(screen.getByRole("button", { name: "Transcribe" }));
    expect(transcribe).toHaveBeenCalledWith("ana@s.whatsapp.net", "v1");
    expect(screen.getByRole("status")).toHaveTextContent("Transcribing…");
    await act(async () => finish(long));
    expect(screen.getByRole("button", { name: long })).toHaveAttribute("aria-expanded", "false");
  });

  it("is collapsed until clicked, and copies", async () => {
    const writeText = vi.fn(async () => {});
    Object.assign(navigator, { clipboard: { writeText } });
    render(<VoiceTranscript m={{ ...voice, transcript: long }} onError={() => {}} />);
    const text = screen.getByRole("button", { name: long });
    fireEvent.click(text);
    expect(text).toHaveAttribute("aria-expanded", "true");
    fireEvent.click(text);
    expect(text).toHaveAttribute("aria-expanded", "false");
    await act(async () => fireEvent.click(screen.getByRole("button", { name: "Copy transcript" })));
    expect(writeText).toHaveBeenCalledWith(long);
  });

  it("follows the message: transcribing elsewhere, then the text", () => {
    const { rerender } = render(<VoiceTranscript m={{ ...voice, transcribing: true }} onError={() => {}} />);
    expect(screen.getByRole("status")).toHaveTextContent("Transcribing…");
    rerender(<VoiceTranscript m={{ ...voice, transcript: "Chego às sete" }} onError={() => {}} />);
    expect(screen.getByRole("button", { name: "Chego às sete" })).toBeInTheDocument();
  });

  it("says why when it can't, and stays out of other messages", async () => {
    const onError = vi.fn();
    transcribe.mockRejectedValueOnce(new Error("Download the model in Settings"));
    const { rerender } = render(<VoiceTranscript m={voice} onError={onError} />);
    await act(async () => fireEvent.click(screen.getByRole("button", { name: "Transcribe" })));
    expect(onError).toHaveBeenCalledWith("Couldn't transcribe the voice note: Download the model in Settings");
    expect(screen.getByRole("button", { name: "Transcribe" })).toBeInTheDocument();
    for (const m of [{ ...voice, kind: "text" }, { ...voice, viewOnce: true }, { ...voice, revoked: true }] as ChatMessage[]) {
      rerender(<VoiceTranscript m={m} onError={onError} />);
      expect(screen.queryByRole("button")).toBeNull();
    }
  });

  it("the setting picks agents' chats or every chat", () => {
    const update = vi.fn();
    const settings = { autoTranscribe: "agents" } as Settings;
    render(<AutoTranscribeField settings={settings} update={update} />);
    const select = screen.getByLabelText("Transcribe incoming voice notes");
    expect(select).toHaveValue("agents");
    fireEvent.change(select, { target: { value: "all" } });
    expect(update).toHaveBeenCalledWith({ autoTranscribe: "all" });
  });
});
