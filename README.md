# wow-addon-cli

A simple addon manager.

**WARNING** back up your existing "AddOns" directory before use. This tool assumes it's the only thing managing your addons. It will delete all non-Blizzard prefixed directories before running by default.

## Usage

Write a `config.toml`, put it in `AddOns` dir
```
# Use this to skip cleanup of specific other directories.
# skipcleanprefixes = ["WOW_HC"]

[[addons]]
git = "https://github.com/hypernormalisation/SwedgeTimer.git"

[[addons]]
git = "https://github.com/bkader/Dominos.git"

[[addons]]
zip = "https://github.com/RichSteini/Bagnon-3.3.5/archive/refs/heads/main.zip"
```

```
# move to addons dir
$ cd AddOns

$ wow-addon-cli
```

Use `-debug` flag for debug logs.

## TODO

- [ ] implement backups

## Other Licenses

`copydir.go`: https://github.com/hashicorp/terraform/blob/v0.13.7/LICENSE

For all else refer to LICENSE
