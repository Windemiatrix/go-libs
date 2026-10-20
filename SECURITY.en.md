# Security policy

**English** | [Русский](SECURITY.md)

## How to report a vulnerability

Use GitHub private reporting: the **Security** tab → **Report a vulnerability**, or <https://github.com/Windemiatrix/go-libs/security/advisories/new>. Do not use a public issue, Discussions or a PR for vulnerabilities: the report is visible only to the maintainer.

Include:

- the module and version (for example, `osxphotos/v0.1.0`), the Go version, the macOS version and the architecture;
- what happens and what damage is possible (writing to the Photos database, reading outside the library, memory corruption in the cgo bridge, data leak);
- reproduction steps on **synthetic** data.

## What you must not attach

The report is stored by GitHub and may become public when the advisory is published. Do not attach:

- `Photos.sqlite` and any copy or export of your library, photos and videos;
- logs and output you have not reviewed: they contain album names, file names, paths and identifiers;
- passwords and tokens.

Instead, build a minimal synthetic database or describe the structure (`IMG_0001.HEIC`, `2024/05-01 - Holidays`) and attach the module version and the Photos database model number (a number, not data).

## Response times

The project is run by one person, so the times are targets: acknowledgement within 7 days, assessment and a fix plan within 30 days. The advisory is published after the fix is released; the reporter is credited if they wish.

## Supported versions

Fixes are released for the latest minor release of the module (before `v1.0.0`, for the latest released version). A faulty version is superseded by the next one with a `retract` directive.

## Scope

In scope: reading and parsing the Photos database and path templates; the cgo bridge to PhotoKit; the supply chain (workflows, releases, dependencies).

Out of scope: vulnerabilities in macOS, Photos and PhotoKit; an attacker with administrator rights on the user's Mac; vulnerabilities in [osxphotos](https://github.com/RhetTbull/osxphotos) itself (report them there).
