# ADR 0143: Device credentials and the KOReader integration

Status: **Proposed; needs human review** · Date: 2026-11-28 ·
Author: Justin + OpenCode

Tracked by [#2868](https://github.com/justin-hayes/mouseion/issues/2868)
(Devices and device credentials; first ticket
[#2869](https://github.com/justin-hayes/mouseion/issues/2869)). Relies on
[ADR 0009](0009-home-lab-auth-corpus-isolation.md). The Calibre push and the
Reading feed of [ADR 0144](0144-push-from-calibre-and-serve-a-reading-feed.md)
accept the credential this ADR defines.

## Context

The learner reads on a Boox Palma 2 running KOReader and wants Mouseion to see
what they read there. KOReader syncs reading progress through the stock kosync
protocol. That protocol sends a username and the MD5 hex of a password in the
`x-auth-user` and `x-auth-key` headers on every request, over plain HTTP on the
tailnet (ADR 0009).

Giving KOReader the account password would store it on the device, and MD5 of a
password is a weak thing to guard it with. Mouseion's sessions are cookies
that a device protocol cannot use.

## Decision

1. **A Device is a credential of its own** beside ADR 0009's sessions. The
   learner names a Device on the Devices page and Mouseion issues it a username
   and a password. Every device protocol accepts the Device credential and
   never the account password, which stays Argon2id and unchanged.
2. **Username.** A slug of the name (ASCII letters and digits joined by single
   hyphens, at most 20 characters, `device` when nothing remains) plus a hyphen
   and a four-character random suffix, for example `palma-k3f9`. It is unique
   across the installation, so a colliding slug gets a new suffix; issuing
   tries a few suffixes and then fails.
3. **Password.** Four groups of four characters joined by hyphens, for example
   `k3f9-m2xq-7bnd-hw4t`, drawn uniformly from `crypto/rand` over lower-case
   letters and digits without the ambiguous `0`, `1`, `i`, `l` and `o`. The
   hyphens are part of the password, because KOReader hashes what the learner
   types.
4. **Storage.** KOReader's wire key is the lower-case MD5 hex of the typed
   password. Mouseion stores only the SHA-256 of that key, as lower-case hex,
   and never the password. A later request is checked by hashing the key it
   sends and comparing in constant time. MD5 here is KOReader's wire format,
   not Mouseion's protection: the stored value is a SHA-256 of a key that
   already comes from a random 20-character password, so a leaked table does
   not reveal the password and cannot be replayed without the MD5 key it
   hashes to.
5. **One-time display.** The password appears once, in the response to adding
   the Device, with `Cache-Control: no-store`. The response is rendered
   directly, not redirected to, so the password is never in a URL, a history
   entry or a later page. A learner who loses it adds another Device.
6. **Name.** Trimmed, non-empty and at most 60 characters; a refusal keeps the
   input in the form.
7. **Accept header.** The `/kosync` surface accepts
   `application/vnd.koreader.v1+json` and does not refuse a missing or
   different `Accept`, which the reference server answers with 412. A KOReader
   client always sends it, so refusing adds nothing.
8. **Integration shape.** KOReader keeps the stock kosync client against
   Mouseion's `/kosync` surface, plus a KOReader plugin for what kosync cannot
   carry. A WebDAV share or exporter files were rejected: they need a second
   credential kind or a manual step each time.
9. **KOReader data is a signal only.** What a Device reports never changes the
   Book, its Study material or its reading history by itself.

## Consequences

- A new `devices` table (owner with cascade, name, unique username, key hash,
  created, last seen and revoked times). Revocation and the kosync surface
  follow in later tickets of #2868.
- The Device credential is a second kind of secret to explain to the learner,
  which the Devices page does at the moment it is shown.
- Plain HTTP means the MD5 key crosses the tailnet readable; the tailnet is the
  boundary, as for sessions.

## Alternatives considered

- **The account password for KOReader.** Rejected: it would sit on the device
  and cannot be revoked on its own.
- **An opaque random token as the Device password.** Rejected: KOReader has
  only a username and password field, and hashes the password it is given.
- **Storing the MD5 key.** Rejected: the stored value would then be the
  credential itself.

## Related

- [ADR 0009](0009-home-lab-auth-corpus-isolation.md)
- [ADR 0144](0144-push-from-calibre-and-serve-a-reading-feed.md)
