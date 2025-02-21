# once

Run the provided command at most once at a time when ran concurrently.

## Usage

```bash
$ once --help

Usage:
  once [flags] <command> [arguments]

The provided command is ran as provided to once with a file-backed mutex
to ensure that other instances of once do not run the same command+args
combination at the same time. When another instance of once fails to lock
the mutex, it will wait until the lock is released. Locks are created
using a hashed version of the command+args to prevent leaking
information about the command being ran.

Flags:
  -h, --help   help for once
```

## Lock Information

Locks are stored at `$XDG_CACHE_HOME/once`. If `XDG_CACHE_HOME` is not
set, the user's home directory is used plus `.cache`.

Example: `$HOME/.cache/once/<sha512>.lock`

## License

GPL-3.0
