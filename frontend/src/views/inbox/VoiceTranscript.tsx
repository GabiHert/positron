import { useEffect, useState } from "react";
import { errText, inbox, type ChatMessage } from "../../api";
import { Icon } from "../../components/ui";

/** VoiceTranscript sits under a voice note or audio message: its transcript, two lines until
 * clicked, with a copy button; or a Transcribe button when it has none yet. */
export function VoiceTranscript({ m, onError }: { m: ChatMessage; onError: (t: string) => void }) {
  const [open, setOpen] = useState(false);
  const [busy, setBusy] = useState(false);
  const [text, setText] = useState(m.transcript);
  const [copied, setCopied] = useState(false);
  useEffect(() => setText(m.transcript), [m.transcript]);

  if (m.kind !== "voice" && m.kind !== "audio") return null;
  if (m.revoked || m.viewOnce) return null;

  if (text) {
    const copy = async () => {
      try {
        await navigator.clipboard.writeText(text);
        setCopied(true);
        setTimeout(() => setCopied(false), 1200);
      } catch (e) {
        onError(errText(e));
      }
    };
    return (
      <div className={open ? "ib-tr ib-tr--open" : "ib-tr"}>
        <button
          type="button"
          className="ib-tr__text"
          aria-expanded={open}
          title={open ? "Show less" : "Show the whole transcript"}
          onClick={() => {
            // Selecting part of the text to copy shouldn't fold it away.
            if (window.getSelection()?.toString()) return;
            setOpen((v) => !v);
          }}
        >
          {text}
        </button>
        <button type="button" className="ib-tr__copy" aria-label="Copy transcript" title={copied ? "Copied" : "Copy transcript"} onClick={copy}>
          <Icon name={copied ? "check" : "copy"} size={13} />
        </button>
      </div>
    );
  }

  if (busy || m.transcribing) {
    return (
      <span className="ib-tr ib-tr--busy" role="status">
        <span className="spinner spinner--sm" aria-hidden="true" /> Transcribing…
      </span>
    );
  }

  return (
    <button
      type="button"
      className="ib-tr__go"
      onClick={async () => {
        setBusy(true);
        try {
          setText(await inbox.transcribe(m.chatJid, m.id));
        } catch (e) {
          onError(`Couldn't transcribe the voice note: ${errText(e)}`);
        } finally {
          setBusy(false);
        }
      }}
    >
      <Icon name="file" size={13} /> Transcribe
    </button>
  );
}
