# wow-addon-cli

## Usage

Write a `config.toml`, put it in `Addons` dir
```
[[addons]]
git = "https://github.com/hypernormalisation/SwedgeTimer.git"

[[addons]]
git = "https://github.com/bkader/Dominos.git"

[[addons]]
zip = "https://github.com/RichSteini/Bagnon-3.3.5/archive/refs/heads/main.zip"
```

```
# move to addons path
$ cd AddOns

$ wow-addon-cli
```

## TODO

- [ ] name config checking
- [ ] implement backups

## Other Licenses

`copydir.go`: https://github.com/hashicorp/terraform/blob/v0.13.7/LICENSE

For all else refer to LICENSE
