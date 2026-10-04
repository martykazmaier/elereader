# Elereader

A full-screen lightbar message reader door for [EleBBS](https://www.elebbs.com/).
It reads and writes EleBBS JAM message bases directly, in the message area the
caller already has selected.

## Features

- Lightbar message list that opens on the first new message, with the page
  filled out to the last message.
- Full-screen reader that draws ANSI art messages as they were meant to look,
  including cursor movement.
- Post, reply, and kill (K) messages. Replies quote the original with its
  paragraph breaks.
- Sysops are asked whether to quote the original's kludge lines (MSGID, PID,
  SEEN-BY, PATH, ...), shown with `@` in place of ^A.
- Echomail and netmail posts carry a MSGID, and replies a REPLY, so network
  dupe checking works. No tearline or origin line is added.
- Upload a message body with a file transfer protocol instead of typing it.
- File attaches: upload files into the area's AttachPath and download them from
  the message.
- Download a message as text (D) or its attached files (F).
- Search the area by Subject, To, From, or a keyword in the body (S). Matches
  are listed on their own page; Esc or Q goes back to the full list.
- Help screen (?).
- Private mail follows EleBBS rules: it is shown only to the sender, the
  recipient, and users with sysop access to the area.
- Read marks and lastread pointers are kept the EleBBS way, in the area's
  `.JLR` file and the message's received flag.

## Requirements

- EleBBS with JAM message bases.
- A DOOR32.SYS drop file.
- Windows: the 32-bit build (`elereader-win32.exe`). The telnet socket handle
  EleBBS passes in DOOR32.SYS can only be used by a 32-bit program.
- Linux: `elereader-linux-386`, `elereader-linux-amd64`, or
  `elereader-linux-arm64`. On Linux the external editor and the transfer
  protocols are not launched yet, so reading, searching, and killing messages
  work, but posting, replying, uploads, and downloads do not.

## Installing

1. Download the binary for your system from
   [Releases](https://github.com/martykazmaier/elereader/releases), or build it
   (see below). On Windows, rename it to `elereader.exe` if you like.
2. Set the `ELEBBS` environment variable to the EleBBS directory (or `RA` if
   `ELEBBS` is not set). Elereader reads `CONFIG.RA`, `MESSAGES.RA`, and
   `PROTOCOL.RA` from there.
3. Add a door to an EleBBS menu that runs Elereader **directly, not from a
   batch file**, with the node directory as the working directory. Elereader
   finds `DOOR32.SYS` and `EXITINFO.BBS` there and opens the caller's current
   message area.

### Editor and transfers

- The external editor is EleBBS's `ExternalEdCmd` from `CONFIG.RA`. If that is
  empty, the `editor=` line in `elereader.cfg` is used, then the `EDITOR`
  environment variable. The editor gets `MSGINF` and `MSGTMP` in the node
  directory, the same as with EleBBS.
- Uploads and downloads use the external protocols in `PROTOCOL.RA`.
- File attaches are stored under the area's AttachPath.

## Keys

| Message list | |
| --- | --- |
| Up/Down, Home/End | Move the lightbar |
| Left/Right, PgUp/PgDn | Previous or next page |
| Enter | Read the message |
| P / R / K | Post, Reply, Kill |
| S | Search |
| ? | Help |
| Q or Esc | Quit, or leave search results |

| Reading a message | |
| --- | --- |
| Up/Down, PgUp/PgDn, Space | Scroll |
| Left/Right, N/B, +/- | Next or previous message |
| R / P / K / S | Reply, Post, Kill, Search |
| D | Download the message as text |
| F | Download the attached files |
| Q or Esc | Back to the list |

In the list, `*` marks new mail and `+` marks someone else's private mail
(only sysops see it).

## Command line

```
elereader [options] [DOOR32.SYS or its directory]
```

| Option | Meaning |
| --- | --- |
| `-door path` | DOOR32.SYS to use (default: the working directory) |
| `-cfg path` | `elereader.cfg` to use (default: beside the program) |
| `-base path` | Open this JAM base instead of the caller's area |
| `-name text` | Area name used with `-base` |
| `-type kind` | Area type used with `-base`: `local`, `echo`, `netmail`, `email` |
| `-local` | Run on the local console with no drop file |
| `-demo` | Open a built-in sample area on the local console |
| `-user name` | User name for `-local` or `-demo` |

## elereader.cfg

Optional, beside the program. Lines starting with `#` or `;` are comments.

```
editor=c:\ele\iceedit.exe
area General=c:\ele\msgbase\general local
```

`area` lines are only used when the caller's area cannot be found from
EleBBS. The type at the end is `local`, `echo`, `netmail`, or `email`.

## Building

Go 1.22 or later.

Windows (must be 32-bit):

```
build.bat
```

Linux:

```
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o elereader .
```

Run the tests with `go test ./...`.
