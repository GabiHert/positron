# Changelog

All notable changes to Positron are listed here, newest first. Every change a user can notice adds
a line under **Unreleased** in the same commit.

## [Unreleased]

### Added
- Each agent run records what it used: tokens, how much came from the prompt cache, and the cost at API prices. Activity shows it on every run, and an agent's page shows its total for the last 7 days. Runs from before this version have no usage.
- Close the window with ⌘W (File → Close Window) or the red button and Positron keeps running in the background: WhatsApp, the agents and the Claude Code connection go on. Open it again from the Dock, the menu bar or by opening the app. ⌘Q quits. A new setting hides the Dock icon while the window is closed.
- A macOS menu with Edit (copy and paste) and Window → Minimize (⌘M).
- Photos open inside Positron instead of in Preview: WhatsApp photos and stickers, and the photos and reply images in Activity. The viewer fits the photo to the window; zoom with a pinch, ⌘+ and ⌘− or a double-click, and drag to move around. The arrow keys step through the chat's other photos. Esc or a click outside closes it. Save…, Copy and Open in Preview are in its top bar. A photo not yet downloaded loads only when you open it.

- Claude Code can download WhatsApp media through Positron's tools: `download_whatsapp_media` saves a message's photo, video, audio, voice note, document or sticker to `~/Downloads/Positron` or a folder you name (fetching it from WhatsApp when needed), optionally with the voice note's transcript. `export_whatsapp_chat` writes a chat, a date range or its last messages, to Markdown or JSON, with its media alongside if asked. Files are only written inside your home folder and never replace another file.
- `read_whatsapp_chat` and `search_whatsapp_messages` now describe each message's media: type, size, file name and whether it can still be downloaded.

### Changed
- While the window is closed Positron sends it no updates, and it reloads what it shows when it opens again.
- New typography, the same as Hadron's: Inter Tight for the interface and headings and Unbounded for the Positron wordmark replace the serif titles and IBM Plex Sans. Code and monospace text use IBM Plex Mono. The fonts are bundled in the app.

### Fixed
- Positron no longer grows by up to a gigabyte after each voice note. Whisper now runs in a helper process that starts for a voice note and exits after 3 idle minutes, handing all of its memory back.
- Sessions on Positron's MCP servers close after being idle (30 minutes for the assistant's tools, 24 hours for Claude Code in other projects). Before, each one stayed in memory for good, about 380 KB per assistant reply or block refresh.
- An agent with no MCP servers chosen no longer starts all of your MCP servers on every run, which saves about 100 MB and a few seconds per run.
- Custom HTML blocks on the Activity board follow the app's current theme, so their text stays readable in light and dark.
- The Activity page no longer slides sideways and cuts off its left edge.
