# Changelog

All notable changes to Positron are listed here, newest first. Every change a user can notice adds
a line under **Unreleased** in the same commit.

## [Unreleased]

### Added
- Each agent run records what it used: tokens, how much came from the prompt cache, and the cost at API prices. Activity shows it on every run, and an agent's page shows its total for the last 7 days. Runs from before this version have no usage.
- Close the window with ⌘W (File → Close Window) or the red button and Positron keeps running in the background: WhatsApp, the agents and the Claude Code connection go on. Open it again from the Dock, the menu bar or by opening the app. ⌘Q quits. A new setting hides the Dock icon while the window is closed.
- A macOS menu with Edit (copy and paste) and Window → Minimize (⌘M).

### Changed
- While the window is closed Positron sends it no updates, and it reloads what it shows when it opens again.
- New typography, the same as Hadron's: Inter Tight for the interface and headings and Unbounded for the Positron wordmark replace the serif titles and IBM Plex Sans. Code and monospace text use IBM Plex Mono. The fonts are bundled in the app.

### Fixed
- Custom HTML blocks on the Activity board follow the app's current theme, so their text stays readable in light and dark.
- The Activity page no longer slides sideways and cuts off its left edge.
