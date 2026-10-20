# Reverse-Engineering Frontend — buzz (block/buzz)

Stand: 2026-10-20. Quelle: upstream-Klon github.com/block/buzz @ `6e6173ac`
(HEAD, 2026-10-19) unter `/tmp/re-work/buzz-src` (Lizenz Apache-2.0,
`LICENSE`). Vorher im lokalen Addon-Repo `/share/projekte/github/buzz-ha-addon/buzz_relay`
verifiziert, dass dort nur ein HA-Wrapper liegt (Dockerfile/README/config.yaml/
run.sh, `buzz_relay/Dockerfile:1-24`): Der Dockerfile klont buzz selbst (Stage 1,
`buzz_relay/Dockerfile:17`).

## 0) VERIFIZIERTER Umfang — Tauri-Shell + zwei React-Apps (Correction zum Auftrag)

Die Annahme „buzz = Tauri-Shell + web/ React-App, UX liegt in web/" ist **so
nicht korrekt**. Real (Beleg: Zeilenzählung `find src -name '*.tsx' -o -name '*.ts' | xargs wc -l`):

| Paket | Umfang | Rolle |
|---|---|---|
| `web/` (`buzz-web`) | 4.192 LoC TS/TSX (20 Dateien > 50 LoC) | **Kleine Companion-SPA**: nur Features `invite/` (Join-Flow) + `repos/` (Repo-Browser), 5 Routen (`web/src/app/routes/`: `index.tsx`, `invite.$code.tsx`, `repos.$repoId.tsx`, `repos.$repoId.blob.$.tsx`, `root.tsx`) |
| `desktop/` (Tauri-Shell) | 181.529 LoC TSX (`find desktop -name '*.tsx' | xargs wc -l`) | **DAS eigentliche Workspace-UI** — Channels, Messages, Presence, Home, Community-Rail, alles |
| `admin-web/` | eigenes Bundle (vgl. `buzz_relay/Dockerfile:34,39,42`) | Admin-Konsole, sekundär |

Konsequenz für uns: buzz-relay served `web/dist` (`BUZZ_ADMIN_WEB_DIR`/`BUZZ_WEB_DIR`,
`buzz_relay/Dockerfile:73-77`) — das ist keine Chat-UI, sondern Join-Landing +
Lese-/Repo-Ansicht. Die Chat-UX von buzz entsteht im Desktop-Paket. Für
Agent Relay (Go-Binary + eingebettete SPA) ist trotzdem das Muster-Wissen aus
`desktop/` wertvoll; wir können die Patterns 1:1 umsetzen, ohne Tauri.

## 1) App-Struktur & Stack (verifiziert)

- Stack: React + **TanStack Router** (file routes), **TanStack Query** v5,
  **TanStack Virtual**, TipTap-Editor, Tailwind CSS v4
  (`desktop/package.json`: `@tanstack/react-query`, `@tanstack/react-router`,
  `@tanstack/react-virtual`, `@tailwindcss/postcss`).
- Einstieg: `desktop/src/main.tsx`; Routen als TS-Module in
  `desktop/src/app/routes/` (`index.tsx`, `agents.tsx`, `channels.$channelId.tsx`,
  `channels.$channelId.posts.$postId.tsx`, `ChannelRouteScreen.tsx`,
  `WorkflowsRouteScreen.tsx`).
- Feature-Ordner unter `desktop/src/features/` (31 Stück): `agents`,
  `admin-console`, `channel-templates`, `channels`, `chat`, `communities`,
  `messages`, `presence`, `profile`, `sidebar`, `home`, `onboarding`,
  `notifications`, `reminders`, `search`, `settings`, `terminal`, `huddle`,
  `forum`, `projects`, `user-status`, `workflows`, `moderation`, …
- Der **App-Shell ist Kommando-Hooks-Komposition**: `AppShell.tsx` bindet ~60
  Feature-Hooks (Unread, Presence, Typing, Shortcut-, Notification-,
  Archiv-Sync-Hooks) an EINE Shell-Komponente
  (`desktop/src/app/AppShell.tsx:1-121`). Unterteilte Komposite:
  `AppShellChannelSurface.tsx`, `AppShellOverlays.tsx`, `AppHuddleShell.tsx`,
  `AppTopChrome.tsx`, `RelayConnectionOverlay.tsx` (alles in `desktop/src/app/`).
  → Lern-Muster: keine Monolith-View; Shell = Orchestrator, der feature-gekapselte
  Hooks subscribt; UI-Komponenten bleiben dümlich.

## 2) Komponenten-Anatomie der Kern-Surfaces

### Sidebar (`desktop/src/features/sidebar/ui/`)
- `AppSidebar.tsx` + `AppSidebarPinnedHeader.tsx`,
  `CommunityRail.tsx` (Community-Umschaltung links als Rail),
  `SidebarSection.tsx`, `SidebarProjectsSection.tsx`,
  `CustomChannelSection.tsx`, `SidebarDnd.tsx` (Drag&Drop-Reihenfolge),
  `SidebarProfileCard.tsx`, `SidebarRelayConnectionCard.tsx` (Verbindungsstatus
  als eigene Card in der Sidebar), `MoreUnreadButton.tsx`,
  `ChannelContextMenu.tsx` (Rechtsklick), `CreateChannelDialog.tsx`.
- Struktur-Prinzip: Abschnitte als eigene Komponenten (Pinned, Channels, DMs,
  Projects) statt einer monolithischen Liste; pro Abschnitt Kontextmenü + DnD.

### Channels (Räume) (`desktop/src/features/channels/`)
- Zustand/Meta getrennt vom Rendering: ~40 kleine `.ts`-Module (jedes mit
  eigenem `*.test.mjs`!) — `dmResurface.ts`, `forcedUnreadStore.ts`,
  `observedUnreadStorage.ts`, `unreadChannelCounts.ts`,
  `readState/readStateManager.ts`, `threadActivityStorage.ts`,
  `rosterFreshness.ts`, `openChannelDirectory.ts` … (Verzeichnislisting
  `desktop/src/features/channels/`). Pattern: **Read-State/Unread als eigene
  Persistenz-Schicht mit Format-Versionierung** (`readStateFormat.ts`).
- Header-Logik gebündelt: `useActiveChannelHeader.ts`, `useChannelPaneHandlers.ts`.

### Message-Liste (`desktop/src/features/messages/ui/`)
- `MessageTimeline.tsx` (933 LoC): virtualisierte Liste (TanStack Virtual),
  Scroll-Manager-Deferral — „scroll manager and the rendered list off the same
  deferred value … You can't scroll to a row that hasn't committed yet"
  (`MessageTimeline.tsx:248-251`) — Deep-Link-Scroll erst nach Commit; Props:
  `unreadCount`, „Event id of the oldest unread top-level message at channel
  open" + per-thread Unread-Counts (`:124-128`); `UnreadPill`/`unreadCountLabel`
  (`MessageTimeline.tsx:21`).
- `MessageRow.tsx` (1.005 LoC) + `MessageHeader.tsx`, `MessageTimestamp.tsx`,
  `MessageAuthorWithIndicators.tsx`, `MessageAgentOwner.tsx`,
  `MessageReactions.tsx`, `MessageActionBar.tsx` (hover-Aktionen),
  `DayDivider.tsx` (Tages-Trenner), `EmojiOnly`-Erkennung
  (`shared/lib/emojiOnly.ts`) für Riesen-Emoji.
- **Threads**: `MessageThreadPanel.tsx`, `MessageThreadRow.tsx`,
  `MessageThreadSummaryRow.tsx`, `MessageThreadTranscript.tsx` + State
  `useThreadReplies.ts`, `useThreadTargetSync.ts` (channels/),
  `threadActivityStorage.ts` — Threads sind „Folge-Events" im Nostr-Log
  (`isThreadReply` über Event-Tags, `useLiveChannelUpdates.ts:14-18`).
- **Diff-Messages**: `DiffMessage.tsx`/`DiffViewer.tsx` — Code-Patches rendern
  als eigenständiger Nachrichtentyp mit expandable Viewer (Rust-Kontrakt
  `git patches`).

### Composer (`desktop/src/features/messages/ui/MessageComposer.tsx`, 1.012 LoC)
Composer = TipTap-Editor + Stack Komponierter Hooks (imports
`MessageComposer.tsx:1-66`) — jedes Verhalten ist EIN Hook:
- `useMentions`, `isMentionCodeContext` (Mention nicht in Code-Spans!),
  `mentionPattern/mentionDisplay/mentionOccurrences/resolveMentionNames` in
  `shared/lib/`.
- `useEmojiAutocomplete` + `EmojiAutocomplete.tsx`, Custom-Emoji-Support.
- `useChannelLinks` (`ChannelAutocomplete.tsx`) — `#channel`-Links.
- Drafts persistiert per Draft-Key (`resolveSentDraftKey`, `draftSubmitKey.ts`),
  Anhänge im Draft persistiert (`MessageComposerDraftImagePersist.test.mjs`).
- Uploads: `useMediaUpload` + **Background-Upload-Store** mit
  Progress-Pill/Overlay (`ComposerUploadProgressPill.tsx`), Cancel beim Verlassen.
- Agent-Adressierung: `useAgentAddressLockPicker`,
  `useAlwaysAddressShortcut` (⇧⌘M), `getPersistentAgentAudienceScope`,
  `useAutoPinMentionedAgents` (gemensannte Agent-Chips bleiben gepinnt),
  `useMentionSendFlow`, `NonMemberMentionDialog` (Mention eines Nicht-Members
  → Invite-Dialog!).
- Typing-Broadcast: `useTypingBroadcast` (Suppression bei Disconnect,
  `relayClientSession.ts:311`: „Disconnected: not worth triggering a reconnect
  for ephemeral typing").
- Reply/Edit-State als Banner überm Composer (`ComposerReplyEditBanner.tsx`).

### Presence (`desktop/src/features/presence/`)
- `hooks.ts`: Query pro Pubkey-Set (`presenceQueryKey`, `hooks.ts:57`), **Live-
  Subscription primär, 60 s-Poll nur Backstop**, Focus-Refetch erst ab 5 min
  Stale (Konstantenblock `hooks.ts:17-34`);
  `PresenceSubscriptionReconciler` (lib/) hält Subscription-Set konsistent;
  OS-Idle-Stufe (`getOsIdleSeconds`, `osIdle.ts`), Heartbeat-TTL
  ( Presence-TTL_SECONDS), Preference `auto|away|offline` in localStorage
  (`hooks.ts:60-80`).
- UI: `features/presence/ui/` (leeres Listing im Klon — UI-Darstellung der
  Presence läuft über Sidebar/Roster-Komponenten); `BotIdenticon.tsx`
  (Identicon für Bots) liegt in `features/messages/ui/`.

### Join-Flow (`desktop/src/features/onboarding/`) + web/invite
- Onboarding = maschineller Stateful-Flow: `machineOnboarding.ts` (State-
  machine), `communityOnboarding.tsx`, Welcome-Kickoff mit Entrance-Anim
  (`useWelcomeKickoffEntrance.ts`), NSEC-Backup-Steps in riesiger Mono-Type
  (`SetupStep.tsx` 1.186 LoC; Tailwind-Font-Tokens `nsec-key`).
- Web-Join: `features/invite/ui/InvitePage.tsx` (409 LoC) +
  `InviteJoinPolicyNotice.tsx` (161 LoC) — Join-By-Code über URL
  (`web/src/app/routes/invite.$code.tsx`), Claim via `useClaimInvite.ts`.

## 3) State-Management

- **Kein Redux/Zustand.** Single Pattern: TanStack-Query-Cache = Store.
  WS-Events patchen den Cache direkt: `useLiveChannelUpdates.ts` meldet sich
  bei `relayClient` an und ruft `mergeTimelineCacheMessages`
  (Query-Cache-Update für `channelMessagesKey`) + `updateChannelLastMessageAt`
  (`useLiveChannelUpdates.ts:1-13`).
- Dedupe/Coalescing: `createTrailingDebounce` — Event-Bursts werden zu einem
  Cache-Write gebündelt (`useLiveChannelUpdates.ts:22-26`,
  `shared/lib/trailingDebounce.ts`).
- Optimistisch: eigene Messages erscheinen sofort im Timeline-Cache.
- „Client = Projektion. The relay is the single source of truth."
  (`ARCHITECTURE.md:7`; entspricht exakt unserem ARCHITECTURE-Punkt 2.)
- UI-Ephemerals (Typing, Draft-Previews) bewusst NICHT über den Cache
  (Separate Subscriptions, z. B. `subscribeToTypingIndicators`,
  `relayClientSession.ts:374`).

## 4) Realtime (WS-Patches)

- Verbindung: `RelayClient` (`shared/api/relayClientSession.ts`, 1.199 LoC) —
  Subscription-API: `subscribeToChannel` mit History-Replay (filter mit
  `…, 50`), `subscribeToChannelLive` („starting now, with no history replay",
  `:336-337`), `subscribeToHuddleEvents`, `subscribeToTypingIndicators`,
  `subscribeToUserStatusUpdates`, `subscribeToAllStreamMessages`.
- **Reconnect-Replay**: nach Disconnect werden Subscriptions re-sent;
  sichtbarer Kanal zuerst („subscriptions for the visible channel are sent in
  the first replay batch so the user sees their active channel recover before
  others") — `relayClient.ts:10-15`; `relayReconnectReplay.ts` (411 LoC);
  Reconnect-Policy `relayReconnectPolicy.ts`, Stall-Watchdog
  `relayStallWatchdog.ts`, Rate-Limit-Gate `relayRateLimitGate.ts`,
  Read-Only-Client-Fallback `readOnlyRelayClient.ts`.
- Rendering-Modell: Event (Nostr-Kind) → Filter-Match (`CHANNEL_EVENT_KINDS`,
  `relayClientSession.ts:1-25`) → Query-Cache-merge → React re-rendert
  virtualisierte Rows. Kein „Token-Streaming" wie bei uns: buzz-Events sind
  final (signed events) — **Streaming-Ausgabe ist bei uns ein Add, das buzz
  nicht braucht** (bei buzz rendert der Agent-Acme-Client, wir rendern
  `agent.token`-Events live; Analogie siehe Transfer-Matrix).
- Message-Anhang: `imetaMediaMarkdown` (imeta-Tags → Media), Spoiler-Particles.

## 5) Interaction-Patterns (kopierenswert)

Keyboard-Shortcuts als **deklarativer Katalog + Datenquelle für das
Shortcuts-Hilfs-Dialog** (`shared/lib/keyboard-shortcuts.ts:24-250`,
Kategorien Navigation/Messages/Formatting/Zoom):
- ⌘K Quick search · ⇧⌘O Browse channels · ⇧⌘K New DM · ⇧⌘N New channel · ⌘, Settings
- ⌘/⌘] Back/Forward · ⌘F Find in channel · Escape = **Mark current read**,
  ⇧Escape = **Mark all as read** (`keyboard-shortcuts.ts:107-121`)
- Enter senden, ⇧Enter Newline, ⇧⌘M „Address agent" (`:167-173`), ⌘E Code,
  ⌘K Link (im Composer).
- Weil Esc auch als „gelesen markieren" dient: UI-Affordance „Escape schafft
  Unread weg" — sehr Slack-übersetzt.

- **Unread-Machine separat** (channels/): observed unread events (browser-lokal,
  „UNREAD_TRIGGER_KINDS"-Menge, `useLiveChannelUpdates.ts:34-40`), Read-Marker
  pro Thread (`unreadReadMarker`, `unreadRootIdStore`), Forced-Unread fürs
  Testen/Debugging, DM-Resurface (versteckte DM kommt wieder hoch, wenn neue
  Msg), `MoreUnreadButton` am Listenende.
- DnD: Sidebar-DnD-Reihenfolge (`SidebarDnd.tsx`) + Drag-auf-Abschnitt zum
  Verschieben.
- Toasts/Dialoge: `ChannelActivityPopover.tsx`, `ConfirmDialog`-Klasse im
  `shared/ui/`.
- Skeletons: `MessageThreadPanelSkeleton.tsx` (Thread-Panel skellettiert).
- Empties: `ChannelIntroBlock.tsx` (Raum-Intro für neue Kanäle),
  `NewMessageScreen.tsx` (DM-Start, `DirectMessageIntroAvatarStack.tsx`).

## 6) Styling / Tokens / Dichte

- Tailwind v4 + CSS-Variablen (shadcn-artiges `card.tsx`/`button.tsx` in
  `web/src/shared/ui/`, Desktop analog).
- **Virtuelle Type-Rem**: sämtliche Schriftgrößen relativ zu
  `--buzz-type-rem` — Cmd/+ zoomt Layout, Font-Size-Pref nudgt nur Text
  (Kommentar `tailwind.config.js:5-14`); Ramp `2xs` (11 px), `3xs` (8 px),
  `badge` (10 px) UNTER text-xs für Meta/Zeitstempel
  (`tailwind.config.js` fontSize-Block); Nachrichten-Schrift als
  `--conversation-message-font-size` + `--conversation-message-line-height`
  Tokens (Tailwind fontSize „message", `tailwind.config.js:19-23`).
  → Guard: „Do NOT reintroduce arbitrary text-[…]px; the px-text guard rejects
  them" (`tailwind.config.js:10-11`) — eslint-Regel erzwingt Token.
- **Richtungs-Schatten**: `shadow-content-edge` = `-1px -1px 0` Hairline +
  Left-edge-Elevation für Flächen, deren sichtbare Kante links ist
  (`tailwind.config.js` boxShadow-Block: „Tailwind's stock shadows are all
  y-offset … -x so they wrap the surface's rounded left corners"). Brauchen
  wir v. a. für unsere Board-Drawer; für die HA-Sidebar eher nicht.
- Density-Settings pro Nutzer: `conversationDensityPreference.ts`,
  `fontSizePreference.ts` (Chat-Dichte als Einstellung, nicht hard-coded).

## 7) Was das buzz-UX ausmacht (Zitat-Belege)

- „A workspace where humans and agents build together, on a relay you own."
  (`README.md:3`); „Humans and agents share the same rooms" (`README.md:14`).
- **Agents = first-class Mitglieder mit eigenem Keypair** — „Agents have their
  own keys, their own channel memberships, and their own audit trail. Scoped
  by identity, not by permission flags — the same way you'd scope a teammate"
  (`README.md:37-39`); „pull in whoever needs to see it. The same affordances
  as a human teammate, the same audit trail, a different keypair"
  (`README.md:24-26`).
- **„Answers with receipts"**: „Ask the project a question and get an answer
  with receipts. Agents search six months of history and post the threads, not
  vibes" (`README.md:31-33`); „Search the conversation, the patch, the
  workflow run, and the approval in one place — because they're all the same
  kind of event" (`README.md:43-46`). → Ein Event-Typ = ein UI-Primitive =
  durchsuchbar. Das ist die buzz-DNA: **Alles ist Ereignis im selben Log und
  damit alles dieselbe UI-Sprache** (Room-Event, Patch, Approval, Reaktion).
- Join-By-URL + Claim-Invite ist der Onboarding-Kern (URL = Workspace,
  `README.md:10-15`).
- Selbst-Treffer: „it's an event log with taste and a suspicious number of
  Rust crates" (`README.md:20`).

## 8) Gut/Schlecht für Plain-JS-Reimplementation (kein Build)

Gut adaptierbar ohne Build: Query-Cache-Substitut (unser `state.messages`-
Objekt + gezieltes Re-Render), Unread-Read-State in localStorage mit
Versions-Key, trailing debounce (~60-Zeiler), Shortcut-Katalog aus Daten,
Composer-Feature-Stufen in kleine Hooks-Module → in plain JS als kleine
Funktionen mit `addEventListener`-Binding.
Nicht nötig/unpassend: TanStack Virtual (unsere Räume sind klein —
„suspicious number of Rust crates"-Level-Optimierung unnötig), TipTap
(contenteditable + eigene Mention-Logik reicht), Tauri-/Desktop-Bindings,
Huddle-Audioworklet.
Ungeklärt: buzz' Token-zoom-Verhalten in HA-Ingress (HA kontrolliert Zoom
selbst); wir entscheiden uns v2.

## Kern-Liste buzz (für Transfer-Matrix)

1. Live-Subscription + Replay: sichtbarer Raum zuerst nach Reconnect.
2. Unread als Persistenz-Layer (Read-Marker pro Thread, observed-unread Events).
3. Composer als Hook-Stack (Mention/Emoji/Draft/Upload/Agent-Address-Flow).
4. Escape/⇧Escape „mark read" + Shortcut-Katalog als Daten.
5. Alles ist Event → Patch in einen Cache → gezielt neu rendern.