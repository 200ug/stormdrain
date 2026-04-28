# stormdrain

Declarative JSON profiles for sandboxed development environments with Podman.

## structure

Generally templates live in `~/.config/stormdrain/`, but practically they can be stored anywhere and pointed at with command line arguments during the creation stage of a new container. The primary config directory's contents include the following:

- `Dockerfile.base` acts as the base template image for all new containers. Placeholders (`{{PROFILE_PKGS}}`, `{{PROFILE_DIRS}}`, `{{PROFILE_INSTALLERS}}`, and `{{PROFILE_CONFIGS}}`) are substituted with commands derived from the active profile.
- `profiles/` contains individual JSON profiles, each describing one environment (packages, toolchains, config mounts, volumes, etc.). See `example_profiles/` for a few basic examples.

This layout can be initialized with `scripts/init.sh`, which populates the aforementioned config directory with the example profiles and the Dockerfile shipped with this repository.

After creating a new profile inside a project directory by running `stormdrain new <profile>` (or `stormdrain new -f <profile_path>`), a `.stormdrain/` directory is created. This is the place where the tool persists the configurations and other metadata specific to that particular project. It's advisable to simply gitignore this. The directory's contents include the following:

- `Dockerfile.sd` is the substituted Dockerfile generated from `Dockerfile.base` and the active profile.
- `pod_spec.json` persists the container configuration (name, project path, image tag, volume mounts, env files, and such) for commands like `enter`, `close`, and `rm` to reference.
- `configs/` is a temporary staging directory for config files copied during the build process. It is cleaned up automatically after container creation.

## profiles

Profiles are the primary way of configuring and templating container environments. Besides apparent metadata like name and description, the following variables are supported:

| Variable | Description | Defaults |
| - | - | - |
| `shell` | Login shell for the container user (`dev` by default) | `/bin/zsh` |
| `packages` | List of APT packages to install during image build | `[]` |
| `installers` | Shell commands executed during image build (by the container user, needs `sudo` for root), basically a way to expand the otherwise structured configuration | `[]` |
| `configs` | Host files/dirs to copy into the image at build time, each entry is `{ "src": <path>, "dst": <path>, "exclude": <pattern> }` | `[]` |
| `project_mount` | Whether to bind-mount the project directory into the container at /home/dev/<project> and set it as the working dir, set to `false` to disable | `true` |
| `ports` | Host-to-container port forwarding. Each entry is `{ "host": <port>, "container": <port> }` | `[]` |
| `virtual_volumes` | Named podman volumes for persistent container-local storage (e.g. caches), each entry is `{ "name": <name>, "path": <path_on_container> }`, volumes are owned by the container user | `[]` |
| `env_files` | Host `.env` files whose key-value pairs are injected as environment variables into the container at runtime | `[]` |

## usage

```
[?] usage: stormdrain <command> [flags]

commands:
  new [-f <path>] <profile>  create a new container from a profile (or profile file path)
  enter [name]               attach a shell to a container matching cwd or given container name
  close [name] [-f]          close the container matching cwd or given container name (optionally SIGKILL)
  rm [name]                  remove the container matching cwd or given container name
  ls [-f <filter>] [-s]      list all stormdrain containers (optional filtering and stats)
  purge                      shut down and delete *all* stormdrain containers
  help                       print this usage message
  version                    print current build version
```

---

###### Mirrors: [Codeberg](https://codeberg.org/2ug/stormdrain) / [Github](https://github.com/200ug/stormdrain)
