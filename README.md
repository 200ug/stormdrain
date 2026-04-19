# stormdrain

Declarative JSON profiles for sandboxed development environments with Podman.

## structure

Persistent configurations live in `~/.config/stormdrain/`:

- `Dockerfile.base` acts as the base template image for all new containers. Placeholders (`{{PROFILE_PKGS}}`, `{{PROFILE_TOOLCHAINS}}`, `{{PROFILE_INSTALLERS}}`) are substituted with commands derived from the active profile.
- `profiles/` contains individual JSON profiles, each describing one environment (packages, toolchains, dotfiles mounts, etc.). See `example_profiles/` for a few basic examples.

This layout can be initialized with `scripts/init.sh`.

Running the tool inside a project directory creates `.stormrdain/` there to hold the generated Dockerfile, compose file, and volume mounts for that environment. This directory is intended to be ignored by git (in most cases handled automatically).

## usage

Planned (approximate) interface:

```
stormdrain new <profile>    # build new image and start container for cwd
stormdrain enter <profile>  # attach a shell to a running container
stormdrain down <profile>   # tear down
```

---

###### Mirrors: [Codeberg](https://codeberg.org/2ug/stormdrain) / [Github](https://github.com/200ug/stormdrain)
