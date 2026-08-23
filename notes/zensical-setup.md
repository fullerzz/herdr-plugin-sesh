# Zensical setup research

## Minimal local site

- `zensical new [OPTIONS] PROJECT_DIRECTORY` scaffolds `zensical.toml`, `docs/index.md`, `docs/markdown.md`, and `.github/workflows/docs.yml`; omitting the directory uses the current directory, and existing files are left untouched. [Zensical: New project](https://zensical.org/docs/usage/new/)
- The current project already declares Zensical as an uv-managed dependency, so the smallest local commands are `uv run zensical new .` followed by `uv run zensical serve`. [Current `pyproject.toml`](../pyproject.toml) [Zensical: New project](https://zensical.org/docs/usage/new/) [Zensical: Setup basics](https://zensical.org/docs/setup/basics/#dev_addr)
- Zensical requires `site_name`; `site_url` should be set to the canonical published URL, while the defaults already use `docs/` as the source directory, `site/` as build output, the modern theme, and `localhost:8000` for the development server. [Zensical: Setup basics](https://zensical.org/docs/setup/basics/#settings)
- Files under `docs_dir` are watched automatically during `zensical serve`, so this site does not need an explicit `watch` setting. [Zensical: Setup basics](https://zensical.org/docs/setup/basics/#watch)

Minimal configuration for this repository:

```toml
[project]
site_name = "herdr-sesh"
site_url = "https://fullerzz.github.io/herdr-plugin-sesh/"
repo_url = "https://github.com/fullerzz/herdr-plugin-sesh"
repo_name = "fullerzz/herdr-plugin-sesh"

nav = [
    { "Overview" = "index.md" },
    { "Configuration" = "config.md" },
    { "Keybindings" = "keybindings.md" },
]
```

This keeps Zensical's documented defaults and mirrors BorgBoi's explicit canonical URL, repository metadata, and flat navigation without copying its optional theme customization. [Zensical: Setup basics](https://zensical.org/docs/setup/basics/) [BorgBoi `zensical.toml`](https://github.com/fullerzz/borgboi/blob/f27ef9b78c4e712587e54acf0f7255ac6ff75ee4/zensical.toml)

## GitHub Pages precedent

- BorgBoi keeps Zensical in an uv dependency group and exposes local preview as `uv run zensical serve`; this project already has the dependency, so moving it into a separate group is unnecessary for the requested local setup. [BorgBoi `pyproject.toml`](https://github.com/fullerzz/borgboi/blob/f27ef9b78c4e712587e54acf0f7255ac6ff75ee4/pyproject.toml) [BorgBoi `justfile`](https://github.com/fullerzz/borgboi/blob/f27ef9b78c4e712587e54acf0f7255ac6ff75ee4/justfile) [Current `pyproject.toml`](../pyproject.toml)
- BorgBoi's Pages job grants `contents: read`, `pages: write`, and `id-token: write`; configures Pages; checks out the repository; installs uv and Python; syncs the frozen lockfile; runs `zensical build --clean`; uploads `site/`; and deploys it with `actions/deploy-pages`. [BorgBoi `.github/workflows/docs.yml`](https://github.com/fullerzz/borgboi/blob/f27ef9b78c4e712587e54acf0f7255ac6ff75ee4/.github/workflows/docs.yml)
- Zensical's generated `.github/workflows/docs.yml` is likewise intended to build and publish the site to GitHub Pages, so BorgBoi is a project-specific refinement of the official scaffold rather than a separate deployment design. [Zensical: New project](https://zensical.org/docs/usage/new/) [BorgBoi `.github/workflows/docs.yml`](https://github.com/fullerzz/borgboi/blob/f27ef9b78c4e712587e54acf0f7255ac6ff75ee4/.github/workflows/docs.yml)

## Practical recommendation

Run the official scaffold once, replace its sample Markdown with a small overview while retaining the repository's existing `docs/config.md` and `docs/keybindings.md`, then verify the local site at `http://localhost:8000/`. Defer theme palettes, icons, Markdown extensions, and the deployment workflow until the local content works; Zensical's defaults already cover the requested basic wiki. [Zensical: New project](https://zensical.org/docs/usage/new/) [Zensical: Setup basics](https://zensical.org/docs/setup/basics/) [BorgBoi `zensical.toml`](https://github.com/fullerzz/borgboi/blob/f27ef9b78c4e712587e54acf0f7255ac6ff75ee4/zensical.toml)
