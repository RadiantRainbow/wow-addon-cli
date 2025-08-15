# wow-addon-cli

A CLI World of Warcraft addon manager.

**WARNING:** back up your Addons dir before using this tool!

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

## How it works

To begin, directories under `AddOns/*` that have a special marker file `.wow_addon_cli` are removed.

For each item in the config, a uuid directory is created in `.downloads` to contain the downloaded file or git repo.

The downloaded item is "unpacked" to a destination in `AddOns/<addon_name>`.

The unpacking process looks for `<addon_name>.toc` files in the downloaded sources. The destination `AddOns/<addon_name>` is determined by the toc file name.

> Addons can contain more than 1 .toc file in their subdirectorires, so only the shallowest `.toc` file is considered as the addon "root" when unpacking

The special marker file `.wow_addon_cli` should exist in each AddOns sub directory that the tool creates.

## TODO

- [ ] implement backups

## Other Licenses

`copydir.go`: https://github.com/hashicorp/terraform/blob/v0.13.7/LICENSE

For all else refer to LICENSE
