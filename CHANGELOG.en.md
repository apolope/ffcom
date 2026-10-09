# Version history

What each published version shipped, newest first. This is the English translation of [`CHANGELOG.md`](CHANGELOG.md), which is written in Portuguese and remains the source: every entry is written there first and then translated here, with the same sections in the same order (same component, version and date). The home page (`https://ffcom.a3sitsolutions.com.br/#versoes`) reads the file for the visitor's language straight from `main`, every deploy workflow rejects a tag that is missing its entry in either file (`scripts/check-changelog.sh`), and GitHub releases (`client` and `channel`) carry both versions, Portuguese first and English below (`scripts/changelog-notes.sh`).

Each component has its own version (independent semver, see `docs/architecture.md`), so the sections below are per tag, not per date.

## Format

One section per published tag, in the exact format below (the site and the CI check depend on it; the header line is identical in both files, only the items are translated):

```
## <component> v<X.Y.Z> · <YYYY-MM-DD>
- What changed, in user terms, one line per item.
```

`<component>` is one of: `client` (the app), `central` (server-central), `channel` (server-channel), `channel-image` (the server-channel container image), `site` (this home page). Items accept `code` in backticks and **bold**. Text outside the sections (like this) is ignored by the site.

## client v0.24.5 · 2026-10-09
- A short network drop during a call no longer kicks you out of the room: the app reconnects on its own, including on Android with the screen off. Meanwhile the call and the notification show "Reconnecting…", and your microphone comes back the way it was.

## client v0.24.4 · 2026-10-09
- In the Android app, muted channels show a 🔕 after their name, and a muted server shows the 🔕 on its icon and at the top of the channel list.

## client v0.24.3 · 2026-10-09
- Phone notifications now arrive even when FFCom is open on your computer in a hidden tab or a minimized window. Only someone with the channel on screen, the window in focus and not away stops getting them.
- In the Android app, a bell at the top of the channel mutes its notifications.
- In the Android app, the update button shows up again when a new version of the app is released.

## channel v0.11.2 · 2026-10-09
- Phone notifications now arrive even when FFCom is open on your computer in a hidden tab or a minimized window: the server only skips people who are actually looking at the channel, with the screen visible, the window in focus and not away. Before, any open channel on any device held the notification back.
- The server log now has one line per message saying how many people push considered, how many were looking at the channel and how many were notified, counts only.

## central v0.15.2 · 2026-10-09
- The server log now has one line per message notice saying how many push tokens arrived, how many were unknown, muted or over the limit and how many became notifications, counts only.

## client v0.24.2 · 2026-10-09
- In the Android app, notifications show the sender's photo, for channel messages, direct messages and friend alerts. People without a photo still get the letter. (v0.24.1, with the same change, was never published.)

## site v0.9.1 · 2026-10-09
- The Privacy section explains that Android app notifications also carry a temporary link to the sender's photo, valid for up to 48 hours.

## client v0.24.1 · 2026-10-09
- In the Android app, notifications show the sender's photo, for channel messages, direct messages and friend alerts. People without a photo still get the letter.

## channel v0.11.1 · 2026-10-09
- Push notifications now tell server-central who sent the message, so the Android app can show that person's photo.

## central v0.15.1 · 2026-10-09
- Push notifications carry a temporary link to the sender's photo, which the Android app downloads without signing in. The link lasts up to 48 hours and only serves that photo. `CENTRAL_PUBLIC_URL` sets the server's public address for building the link (empty: the official instance).

## site v0.9.0 · 2026-10-09
- "Download for Android" button, with step-by-step instructions for installing the APK outside the Play Store and a link to every version of the app on GitHub, with the notes for each one.
- New Privacy section, in the footer: what the official instance stores and where each thing goes, including the Android app notifications through Google's Firebase.
- The app demo on the home page now has the same design as the app's text channel.

## client v0.24.0 · 2026-10-09
- FFCom now has an Android app: download the APK with the "Download for Android" button on the site. The app itself tells you when there is a new version and installs it with one tap.
- In the Android app, "Sign in" opens the login in the phone's browser and comes back to the app on its own.
- Invite links are now `https://app.ffcom.a3sitsolutions.com.br/convite?...` addresses: on a phone with the app, they open straight in the app; in the browser and on desktop, they open "Add a server" already filled in, even if you need to sign in first. Old invite links still work.
- In the Android app, a voice call keeps going with the screen off or another app open, with an "In a call" notification showing the call time and buttons to mute and to leave.
- In the Android app, you get notifications for new messages in channels, direct messages and friend requests, grouped by channel. Tapping one opens the channel, the conversation or Friends.
- "Mute notifications" in the server menu and in each channel's menu, to stop getting alerts from where they don't matter. The choice is saved to your account.
- Anyone with the new "Move members between voice rooms" permission can drag a person, in the sidebar, from one voice room to another.
- Warnings about a microphone that is denied, missing or in use by another app show up in your language, and you join the room listen-only instead of being left out.

## channel v0.11.0 · 2026-10-09
- Push notifications in the Android app: each member hands the server a push token, which is removed along with the person on a kick or ban, and on each new message the server alerts, through server-central, everyone who can see the channel, except the author and whoever has the channel open. `FFCOM_CENTRAL_URL` picks the server-central (empty: the official instance; `off`: turns push off).
- New "Move members between voice rooms" permission (`MoveMembers`) and the `POST /api/voice/move` route, to move someone to another voice room.

## central v0.15.0 · 2026-10-09
- Push notifications for the Android app through Firebase Cloud Messaging: device registration, a push token per server in the account's list, muting per server or channel saved to the account, and alerts for direct messages (with only the sender's name) and for friend requests and acceptances. Message text passes through the server only in transit, without being stored.

## client v0.23.0 · 2026-10-08
- In a call, whoever is speaking gets a green ring: around the microphone icon on the call screen, and around their avatar in the room list in the sidebar.
- The list of who is in each voice room, in the sidebar, no longer reorders itself: people are listed alphabetically.
- Messages in text and forum channels show the sender's name instead of part of their id.
- Hovering over a message with an image no longer opens an empty gap below it: the Edit and Delete buttons appear on top, in the corner.

## site v0.8.0 · 2026-10-03
- The site now has an English version: it opens in your browser language (Portuguese for Portuguese speakers, English for everyone else) and you can switch it in the footer.
- The version history, the sign-up form and the ideas area also show up in English, error notices included.
- A sign-up request made in English gets the set-your-password email in English.

## channel v0.10.0 · 2026-10-03
- API and WebSocket errors come with a code (`{"code", "message", "params"}`), which the app translates into the user's language. Older apps keep seeing the message in Portuguese.

## client v0.22.0 · 2026-10-03
- You can now unfriend someone: right-click the friend in the Friends list and choose "Remove friend". They disappear from your list and you from theirs, and the open conversation closes on both sides. The messages are kept and show up again if you become friends again.
- FFCom now speaks English: under "Language", in your avatar menu, pick Portuguese or English and the interface switches right away, without reloading. The choice is saved to your account and applies on all your devices; with no choice, the app follows the system language.
- Dates, times and numbers use the format of the chosen language.
- Error notices from the servers show up in your language, instead of always in Portuguese.
- The sign-in and password recovery pages open in the app language.
- In the desktop app, the English menu bar (File, Edit, View...) is gone on Windows and Linux; copy, paste, undo and zoom (Ctrl with +, - and 0) keep working. On macOS, the menu follows the app language.

## central v0.14.0 · 2026-10-03
- Unfriending (`DELETE /api/friends/{accountId}`), by either side, notifying both right away. After that, either one can send a friend request again.
- The chosen language is saved to the account (`PATCH /api/me` with `language`, returned by `GET /api/me`), so it applies on all devices.
- API and WebSocket errors come with a code (`{"code", "message", "params"}`), which the app translates into the user's language. Older apps keep seeing the message in Portuguese.
- The sign-up request stores the site language, and the set-your-password email, sent on approval, goes out in that language.

## client v0.21.0 · 2026-10-02
- The voice call no longer drops when you open a text channel, a forum, another server or Friends: "Voice connected" shows at the bottom of the channel list, with the call's channel, the mute button, the leave button and a shortcut back to it.
- The mute and **push-to-talk** shortcuts work with any screen open during the call.
- In the desktop app, "Sign in" opens the login page in your browser, with the account and passwords you already saved there, and returns to the app on its own. After "Sign out", the next "Sign in" asks for the password, so you can switch accounts.

## client v0.20.0 · 2026-09-30
- Someone who was kicked from a server and still has it in their list sees the notice "You are no longer a member", with the option to rejoin by pasting a new invite or to remove the server from the list, instead of an empty screen. This also applies to someone kicked while the server is open.

## channel v0.9.0 · 2026-09-30
- Private channels work like on Discord: denying "View channel" for @everyone and allowing it for a role opens the channel to whoever has that role (before, only Administrators and the owner could get in). Allowing on one role also beats Deny on another role of the same person.
- Someone who manages roles without being an Administrator can no longer delete, edit, remove from someone, or change the channel permissions of a role with permissions they don't have, and nobody can kick or ban someone with permissions they don't have.

## client v0.19.1 · 2026-09-30
- The channel permissions window explains how to make a private channel: deny "View channel" for @everyone and allow it for the roles that get in.

## client v0.19.0 · 2026-09-30
- The invite link carries the server's name: whoever pastes the link already sees the name filled in, and everyone calls the server the same way.

## client v0.18.0 · 2026-09-30
- **Sound when someone joins or leaves the call**, including yourself: three rising notes when joining and falling ones when leaving. You can turn it off in "Sound on join and leave", on the voice channel bar.

## client v0.17.7 · 2026-09-29
- In the desktop app, the green button's tooltip explains that it closes, installs the new version and opens again, instead of talking about reloading the page.

## site v0.7.0 · 2026-09-29
- **Download for Windows** button at the top, always with the latest version of the desktop app, and an explanation of the Windows warning during installation.

## client v0.17.6 · 2026-09-29
- **Windows app published**, with an installer to download from the site.
- The desktop app shows when there is a new version through the same green button as the browser: one click installs and reopens.

## client v0.17.5 · 2026-09-29
- Desktop app: **push-to-talk works with FFCom in the background**, for example with a game in focus, without blocking the key in other programs.

## client v0.17.4 · 2026-09-29
- On mobile, Android's "back" button closes the open drawer instead of leaving the app.

## central v0.13.1 · 2026-09-29
- The desktop app is always accepted by the server, with nothing to configure.

## channel v0.8.1 · 2026-09-29
- The desktop app is always accepted by the server, with nothing to configure: whoever hosts their own server doesn't need to touch the `.env` for the installed app to work.

## client v0.17.3 · 2026-09-29
- The avatar menu shows the app version, so you know which version each device is on.
- Desktop app: login returns to the app instead of stopping on a screen with a loading error.

## client v0.17.2 · 2026-09-29
- Debug key to test the voice connection through the TURN server only (`ffcom:forceRelay` in the browser's local storage); without it, nothing changes.

## client v0.17.1 · 2026-09-29
- The green update button shows up again when two versions come out in a row with the app open; before, if the first one didn't finish downloading, the following ones only appeared after reloading the page.

## client v0.17.0 · 2026-09-29
- **Login screen with the FFCom logo**, and "Sign in" leads to a login page also branded FFCom, instead of the generic Authentik branding. Anyone already logged in signs in once more after this version.
- The "Signing out…" screen shows the logo and a progress indicator.
- After setting the password through "Forgot my password", the app signs you in without asking for the password again.

## channel v0.8.0 · 2026-09-29
- Accepts the login done through the FFCom page on Authentik (`auth.ffcom`), besides the old address: `OIDC_ISSUER_URL` can list more than one issuer, separated by commas.

## central v0.13.0 · 2026-09-29
- Accepts the login done through the FFCom page on Authentik (`auth.ffcom`), besides the old address: `OIDC_ISSUER_URL` can list more than one issuer, separated by commas.

## client v0.16.0 · 2026-09-28
- **Display name:** "Change display name" in your avatar menu picks the name that friends, direct messages and servers see; each server's nickname still takes precedence over it. Leaving it empty goes back to your account name.
- **On mobile**, the chat takes the whole screen: servers and channels open in a drawer on the left (☰ or swiping) and the member list in a drawer on the right.
- When signing out, the screen shows "Signing out…" until the end, without the sign-in button that led back to the same account; the sign-in button is disabled while the login opens.
- "Forgot my password" on the login screen, to set a new password and return to the app.
- **Desktop app:** screen sharing now works, with a picker of screens and windows as thumbnails and, on Windows, the option to share the computer's audio. The app got the FFCom icon, and external links (source code, licenses, password recovery) open in the browser.
- Images and files in messages are no longer downloaded again every hour, when the session is renewed.

## central v0.12.0 · 2026-09-28
- New route `PUT /api/me/display-name` to choose the account's display name (empty goes back to the Authentik name); `GET /api/me` now returns `customDisplayName` when a name has been chosen.
- The set-password email for an approved sign-up goes out with the FFCom branding and leads to an FFCom page that ends back in the app.

## site v0.6.0 · 2026-09-28
- "Forgot my password" link right below "Sign in to FFCom".

## channel-image v1.1.0 · 2026-09-28
- The server image now also ships for **linux/arm64** (Raspberry Pi, ARM VPS); `docker pull` picks the machine's version on its own.
- The launcher shows third-party licenses with `--licenses`.

## client v0.15.1 · 2026-09-28
- Typing the invite link in "Add a server" no longer cuts the code at the first character; pasting the link still fills in the address and the code at once.

## client v0.15.0 · 2026-09-28
- Server roles can now be edited after they are created: "Edit" on each role in "Manage members" shows the permissions to turn on and off, including on **@everyone** (for example, "Create invites" for everyone).
- Enter in the name field creates or saves the category and the channel, without having to click the button.

## site v0.5.0 · 2026-09-27
- Whoever already has an account on another A3S infrastructure service ticks "I already have an account" on the form and requests only access, without choosing a username or nickname; once approved, they sign in to FFCom with their usual username and password.
- The form warns as you type if the username is already taken or reserved.

## central v0.11.0 · 2026-09-27
- Requests from people who already have an Authentik account: approval only grants FFCom access to the existing account, without creating another one or sending a password email. The approval message warns, from the moment it is sent, when the request's email already belongs to an account.
- Usernames are checked case-insensitively, and names like `admin`, `root` and `suporte` are reserved.
- New public route `GET /api/signup-requests/username-available` so the form can check the name while the person types.

## site v0.4.0 · 2026-09-27
- New "Join" section to request an account on the official instance: name, username, email, nickname and the reason. Before and after submitting, the page warns that without your own server-channel or an invite only direct messages are available.
- FFCom is now published under the AGPL-3.0 license, with a link to it in the footer and in the "Where we are" section.

## central v0.10.0 · 2026-09-27
- Sign-up requests from the home page: each request reaches the team's Telegram group with approve and reject buttons. Once approved, the account is created with FFCom access already granted, the chosen nickname becomes the displayed name, and an email arrives to choose the password.
- The binary includes the licenses of third-party components, which `--licenses` prints.

## site v0.3.0 · 2026-09-27
- Moderators can delete ideas in any tab, confirming with a second click, and send an implemented idea back to the ranking.

## central v0.9.0 · 2026-09-27
- Moderators can delete an idea for good, along with its votes. Unlike rejecting, deleting gives the author back the day's suggestion, if the idea is from today.

## site v0.2.2 · 2026-09-26
- When the text is not an improvement suggestion, the wand shows what is missing without changing the text, and a discarded submission goes back to the field with the explanation, to be rewritten.

## central v0.8.0 · 2026-09-26
- The wand and the final check now verify that the text really is an improvement suggestion. A generic opinion ("bad app"), a test or a question without a proposal is not published: the person gets a hint about what was missing and can rewrite it, without losing the day's suggestion (up to 3 discards per day).
- A complaint about a concrete problem (e.g. audio cutting out) still counts and becomes the request to fix it.

## site v0.2.1 · 2026-09-26
- Fixes the "Sign in to suggest" button, which did not lead to the login: the page no longer fetches the Authentik configuration through the browser, which was being blocked.

## site v0.2.0 · 2026-09-26
- "Ideas" section: improvement suggestions with login to the FFCom account, a public ranking by score (a like is worth 2, a dislike takes 1 away) and a tab of implemented ideas linked to the version history.
- A wand that improves the suggestion's text with Claude, up to 3 times a day, showing similar existing ideas to vote on.
- Moderation for the administrators group: approve or reject held ideas and mark ideas as planned or implemented.

## central v0.7.0 · 2026-09-26
- Improvement suggestions: one per person per day, votes with like and dislike, and planned, implemented and rejected states.
- Wand and final content check via Claude: offensive words are replaced, an offensive suggestion in the wand is discarded and costs 2 extra uses, and in the final check it goes to moderation.
- The Authentik group `ffcom-admins` moderates the suggestions.

## site v0.1.0 · 2026-09-26
- First version of the home page at `ffcom.a3sitsolutions.com.br`: what FFCom is, what already works, how it works, how to host a server and the story behind the name.
- New project brand: two "f"s joined by the same bar inside a speech bubble.
- Version history read straight from the project's `CHANGELOG.md` on GitHub.

## client v0.14.1 · 2026-09-26
- New icon: favicon and installed app (PWA) icons with the FFCom brand.

## client v0.14.0 · 2026-09-26
- Text chat and forum reconnect on their own when the server restarts or changes version, and recover the messages missed in between.
- Renewing the login no longer drops the channel's open connection.

## channel v0.7.1 · 2026-09-26
- First real automatic update of the official instance: the server changed version on its own, without recreating the container.

## channel-image v1.0.0 · 2026-09-26
- First "evergreen" server image (`ghcr.io/apolope/ffcom-channel:1`): the container updates itself, checks the signature and hash of each version and rolls back to the previous one if the new one doesn't start.
- Works on first boot even without access to GitHub, with a copy of the service built into the image.

## channel v0.7.0 · 2026-09-26
- The server is now published as a signed binary, in a version index that the containers query to update themselves.
- Hot version swap: chat connections drop for a few seconds and voice doesn't drop.

## client v0.13.0 · 2026-09-26
- Your own avatar menu shows name, username, email and nickname.

## client v0.12.0 · 2026-09-26
- The key for encrypted direct messages is stored in the account, protected by a recovery phrase: you can switch browsers or devices without losing your conversations.

## central v0.6.0 · 2026-09-26
- Stores the account's end-to-end key, encrypted by the recovery phrase.

## central v0.5.1 · 2026-09-26
- Friend requests and the friends list show the person's name instead of an internal identifier.

## client v0.11.0 · 2026-09-26
- Send friend requests straight from a server's member list, with a notification and a section for received requests.

## channel v0.6.0 · 2026-09-26
- Request limit per user and per device, instead of per IP address.

## central v0.5.0 · 2026-09-26
- Friend requests (send, accept, decline) with real-time notification for both sides.
- Request limit per user and per device.

## client v0.10.0 · 2026-09-25
- Reorder servers, categories and channels by dragging.
- Standardized colors and scrollbars across the whole app.

## channel v0.5.0 · 2026-09-25
- Order of categories and channels stored on the server, adjustable by dragging.
- Request limit per user.

## central v0.4.0 · 2026-09-25
- Server list order stored in the account.
- Request limit per user.

## client v0.9.0 · 2026-09-23
- Avatar and presence status in the member and friend lists.
- Avatar cropping before upload, with a preview of the result.

## channel v0.4.0 · 2026-09-23
- The member list brings the profile's avatar and name when the person has no nickname.

## central v0.3.0 · 2026-09-23
- Avatar and presence status available to the app's lists.

## client v0.8.1 · 2026-09-23
- Sound when pressing and releasing the push-to-talk key.

## client v0.8.0 · 2026-09-23
- Stronger microphone noise suppression (RNNoise).
- Push-to-talk as an alternative to an open microphone.
- Per-person volume and "mute for me" in the voice channel.

## client v0.7.0 · 2026-09-23
- Configurable keyboard shortcut to mute the microphone, with an audible cue.
- The Invite button only shows for those who can invite.

## channel v0.3.0 · 2026-09-23
- Permission to create invites separate from the one to manage invites.
- Who is in each voice room shows in the sidebar.
- TURN server passed on to clients through LiveKit, so calls can get through more restrictive networks.

## client v0.6.2 · 2026-09-23
- Applying the app update always reloads the page, even on the first visit.

## client v0.6.1 · 2026-09-23
- Paperclip button to attach a file in the chat.
- "Enable audio" notice when the browser blocks the call's audio.
- Copy button always visible in the invite dialog.

## client v0.6.0 · 2026-09-23
- Button to update the app when a new version comes out, without reloading on its own in the middle of a call.

## client v0.5.1 · 2026-09-23
- Fix for the audio shared along with the screen.

## client v0.5.0 · 2026-09-23
- Share audio along with the screen.

## client v0.4.1 · 2026-09-22
- The server header buttons wrap to a new line instead of cutting off the last one.

## client v0.4.0 · 2026-09-22
- Enlarge a person's video or a screen in the voice channel.
- Unread indicator on the servers in the sidebar.
- Per-channel permissions for each role.
- Who is in each voice room shows in the sidebar.
- Administrators can delete other members' messages.

## client v0.3.0 · 2026-09-22
- Sign out button.
- Create, rename and delete categories and channels.

## channel v0.2.0 · 2026-09-22
- Create, rename and delete categories and channels, with its own permission.

## central v0.2.0 · 2026-09-22
- End-to-end key and "read" markers kept per account, so two accounts in the same browser don't get mixed up.

## client v0.2.1 · 2026-09-22
- The selected channel no longer jumps back to the first one on every server structure update.

## client v0.2.0 · 2026-09-22
- Camera in the voice channel.

## channel v0.1.1 · 2026-09-22
- Fix for the address announced for voice media, which kept calls from connecting on the official instance.

## client v0.1.1 · 2026-09-22
- Republication of 0.1.0, which had been blocked by a false positive in the secrets check.

## client v0.1.0 · 2026-09-22
- First published version of the app: login, servers, text, voice and forum channels, friends and direct messages.
- End-to-end encrypted direct messages.
- Edit and delete messages, attach files and images, unread indicator.
- Account avatar, kicking and banning members.
- Works in the browser, installable as an app (PWA), and packageable for desktop (Windows, macOS, Linux).

## channel v0.1.0 · 2026-09-22
- First published version of the server: categories, text, voice and forum channels, roles and permissions, invites.
- Attachments in messages, editing and deleting, kicking and banning, request limiting.
- Connections require HTTPS outside localhost.

## central v0.1.0 · 2026-09-22
- First published version of the central server: account, profile, friends, server list and end-to-end encrypted direct messages.
- Avatar upload, unread indicator and request limiting.
