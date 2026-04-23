# stormdrain

Declarative JSON profiles for sandboxed development environments with Podman.

## structure

Templates live in `~/.config/stormdrain/`:

- `Dockerfile.base` acts as the base template image for all new containers. Placeholders (`{{PROFILE_PKGS}}`, `{{PROFILE_INSTALLERS}}`, and `{{PROFILE_DOTFILES}}`) are substituted with commands derived from the active profile.
- `profiles/` contains individual JSON profiles, each describing one environment (packages, toolchains, dotfile mounts, volumes, etc.). See `example_profiles/` for a few basic examples.

This layout can be initialized and populated with the included samples via `scripts/init.sh`.

Running the tool inside a project directory creates `.stormdrain/` (gitignored):

- `Dockerfile.sd` is the substituted Dockerfile generated from `Dockerfile.base` and the active profile.
- `pod_spec.json` persists the container configuration (name, image tag, shell, mounts, volumes) for commands like `enter`, `close`, and `rm` to reference.
- `dots/` is a temporary staging directory for dotfiles copied during the build process. It is cleaned up automatically after container creation.

## usage

```
[?] usage: stormdrain <command> [flags]

commands:
  new <profile>             create a new container from a profile
  enter [name]              attach a shell to a container matching cwd or given container name
  close [name] [-f]         close the container matching cwd or given container name (optionally SIGKILL)
  rm [name]                 remove the container matching cwd or given container name
  ls [-f <filter>] [-s]     list all stormdrain containers (optional filtering and stats)
  purge                     shut down and delete *all* stormdrain containers
  help                      print this usage message
  version                   print current build version
```

---

###### Mirrors: [Codeberg](https://codeberg.org/2ug/stormdrain) / [Github](https://github.com/200ug/stormdrain)
