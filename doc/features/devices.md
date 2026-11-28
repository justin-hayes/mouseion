# Devices

Status: **Implemented (adding and listing)** · Date: 2026-11-28

## Motivation

The learner reads on a KOReader e-reader and wants Mouseion to see that
reading without putting the account password on the device. A **Device** is a
named credential with its own username and password that device protocols
accept instead (ADR 0143, spec #2868).

## Goals

- Add a Device by name from **Devices** in the account panel.
- Show its generated username, its password and the `/kosync` server address
  once, right after adding it. The password is never shown again; a lost one
  means adding another Device.
- List the learner's Devices with name, username, when each was added and when
  it was last seen (Never until it first syncs). A learner sees only their own.

## Behaviour

- The name is trimmed, non-empty and at most 60 characters. A refused name
  keeps what was typed.
- The username is a slug of the name plus a short random suffix, such as
  `palma-k3f9`. The password is four groups of four lower-case letters and
  digits, such as `k3f9-m2xq-7bnd-hw4t`, without easily confused characters.
- Only a SHA-256 of KOReader's MD5 key is stored; the password cannot be
  recovered from Mouseion.
- The server address is `http://<host>/kosync`, from the host the learner is
  using. Mouseion is meant for the tailnet over plain HTTP (ADR 0009).
- The page and the add response are served with `Cache-Control: no-store`.

## Not in this feature yet

Signing in from KOReader, the KOReader setup steps and revoking a Device are
later tickets of #2868.
