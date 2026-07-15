## v0.1.3 (2026-07-15)

### 🐛 Bug Fixes

- **ci**: Skip unsupported private attestations ([56e6c69](https://github.com/fullerzz/herdr-plugin-sesh/commit/56e6c69b8050d59b4f936af723c0d0840f6aa047))

- **go-deps**: Update go dependencies to v2 ([#33](https://github.com/fullerzz/herdr-plugin-sesh/issues/33)) ([0ffb140](https://github.com/fullerzz/herdr-plugin-sesh/commit/0ffb140f9503132ebcfcdca3d9d6a5f1cb3882e7))

- **justfile**: Push release commit and tag atomically ([ddaaab6](https://github.com/fullerzz/herdr-plugin-sesh/commit/ddaaab61a0e5d8c53f077fa5bd625db403800edb))



### 📚 Documentation

- **README**: Refine readme ([0540744](https://github.com/fullerzz/herdr-plugin-sesh/commit/0540744585d361e22de8c89d79f8107d83628c50))



### ⚙️ Miscellaneous Tasks

- Configure Renovate ([#31](https://github.com/fullerzz/herdr-plugin-sesh/issues/31)) ([a991cab](https://github.com/fullerzz/herdr-plugin-sesh/commit/a991cab94dcc11384f2a9c177ff1e3ad65f59cdf))

- Remove connect-root action from plugin manifest ([3bfd63e](https://github.com/fullerzz/herdr-plugin-sesh/commit/3bfd63e6032e4b03dc40bc1af7cd4b868064266a))

- **justfile**: Add release recipe to justfile ([92d8090](https://github.com/fullerzz/herdr-plugin-sesh/commit/92d8090396904634addd8601d3351f774cdfa777))

- Release v0.1.2 ([1098d09](https://github.com/fullerzz/herdr-plugin-sesh/commit/1098d098ea04bc090f930e02ce88cbd080e17efa))

- Release v0.1.3 ([236b917](https://github.com/fullerzz/herdr-plugin-sesh/commit/236b917f4875399fd6013a408e67794fcdc09216))


## v0.1.1 (2026-07-15)

### 🐛 Bug Fixes

- **ci**: Allow changelog pull request reads ([bb738a2](https://github.com/fullerzz/herdr-plugin-sesh/commit/bb738a2f8978a8cb0281b90fe79cbfb6975c9176))

- **ci**: Generate release notes from full history ([0065a48](https://github.com/fullerzz/herdr-plugin-sesh/commit/0065a48f9952020dd9ab3e843b0a2f8f86be82c0))

- Enforce plugin release version consistency ([#32](https://github.com/fullerzz/herdr-plugin-sesh/issues/32)) ([4b19ef1](https://github.com/fullerzz/herdr-plugin-sesh/commit/4b19ef1fa996da5c75fc07bbbf84164ae2db1fea))



### ⚙️ Miscellaneous Tasks

- Release v0.1.1 ([324aa76](https://github.com/fullerzz/herdr-plugin-sesh/commit/324aa76a145ffffddeded26f78a3344f53890562))



### 🎡 Continuous Integration

- Persist changelog on release tags ([c9d80e1](https://github.com/fullerzz/herdr-plugin-sesh/commit/c9d80e1ead22ad79a056936016732dee07b45e54))

- Attest release build provenance ([a525211](https://github.com/fullerzz/herdr-plugin-sesh/commit/a52521133d94d000b025f4caf017fa4dc9188e23))


## v0.1.0 (2026-07-14)

### 🚀 Features

- Scaffold herdr sesh plugin ([1488f49](https://github.com/fullerzz/herdr-plugin-sesh/commit/1488f49cd25768d3da52320e8f5281cc8eb0abd2))

- Cache session list when enabled ([8545b2d](https://github.com/fullerzz/herdr-plugin-sesh/commit/8545b2dd4a0459ee27061717ebf29af4476437eb))

- Add interactive picker command ([2b7ac19](https://github.com/fullerzz/herdr-plugin-sesh/commit/2b7ac19ba7287b639d93f26a1d347a19c8e376da))

- Add styled session picker ([4e76ff0](https://github.com/fullerzz/herdr-plugin-sesh/commit/4e76ff0aeb9a14133a73f3627ad7287672e8bf35))

- Add source badges to picker view ([f47fed2](https://github.com/fullerzz/herdr-plugin-sesh/commit/f47fed246074b57fb15a02a6097911c0411b7511))

- Add fzf-backed picker and workspace path lookup ([064f457](https://github.com/fullerzz/herdr-plugin-sesh/commit/064f4579820fcfed8d72446f316b6930ea7f2451))

- Add bat-backed preview pane to picker ([746beef](https://github.com/fullerzz/herdr-plugin-sesh/commit/746beefb1d4cdf33f4a1d9b9389aebcaf826f0fc))

- Default preview to eza icons ([142caaf](https://github.com/fullerzz/herdr-plugin-sesh/commit/142caaf72aa0dcb0970e448c1f05f70bde530919))

- Color picker source badges ([656b881](https://github.com/fullerzz/herdr-plugin-sesh/commit/656b881a85cca19918cebba5517056d5147fcb49))

- Add previous workspace switching ([#2](https://github.com/fullerzz/herdr-plugin-sesh/issues/2)) ([9567763](https://github.com/fullerzz/herdr-plugin-sesh/commit/956776356c2f5ed10fdd6e496b1e5efe63000184))

- Add plugin install script ([#29](https://github.com/fullerzz/herdr-plugin-sesh/issues/29)) ([9c7af88](https://github.com/fullerzz/herdr-plugin-sesh/commit/9c7af88e99a955bd876a823fd3a32c7d99f00d54))



### 🐛 Bug Fixes

- Expand configured session paths ([2e38b2f](https://github.com/fullerzz/herdr-plugin-sesh/commit/2e38b2f9704492a6559ae355d92476b32f025d60))

- Return herdr JSON decode errors ([bb2f096](https://github.com/fullerzz/herdr-plugin-sesh/commit/bb2f096c73597efb3232c6d76672e5cd07e3be8e))

- Make plugin state writes resilient ([1450e38](https://github.com/fullerzz/herdr-plugin-sesh/commit/1450e38272f29a630cd899883775b0854df4178d))

- Tighten file permissions and handle write errors ([85ce90c](https://github.com/fullerzz/herdr-plugin-sesh/commit/85ce90c0615318836e5a3d42671521828b5862b9))

- Accept wrapped herdr JSON envelopes ([7bf2b66](https://github.com/fullerzz/herdr-plugin-sesh/commit/7bf2b6609fcc2b2d299eecf2a98eb0ce6e91bc0e))

- Keep picker highlight active in selected rows ([f0220de](https://github.com/fullerzz/herdr-plugin-sesh/commit/f0220ded81256c2760a02c1dfed65e934d3e3871))

- Handle bare JSON arrays in herdr responses ([32beacb](https://github.com/fullerzz/herdr-plugin-sesh/commit/32beacba161ca68990efd71f5e4517120a847664))

- Reset picker selection when query changes ([23c2f92](https://github.com/fullerzz/herdr-plugin-sesh/commit/23c2f923cc209b7629bdd4021000b9bac7bba068))

- Use configured preview command in picker ([27f1e49](https://github.com/fullerzz/herdr-plugin-sesh/commit/27f1e499b361a05d0a70d5e92c3a6a965cee430b))

- Preserve fzf preview path lookup ([ee07876](https://github.com/fullerzz/herdr-plugin-sesh/commit/ee078765c71858264818a8262d42cb46b98b5f68))

- Pad native picker top border ([f3f9e0e](https://github.com/fullerzz/herdr-plugin-sesh/commit/f3f9e0ed4ec99507e39a0b5cca35dce4366a86fe))

- Resolve clone destination under cmdDir ([#17](https://github.com/fullerzz/herdr-plugin-sesh/issues/17)) ([0d07c12](https://github.com/fullerzz/herdr-plugin-sesh/commit/0d07c12b516aaeb903fd712b5e0ecb55f2141af5))

- Package release binary under manifest path ([#20](https://github.com/fullerzz/herdr-plugin-sesh/issues/20)) ([1430414](https://github.com/fullerzz/herdr-plugin-sesh/commit/14304141afbe698a0097f0efa2f6908c4558e4ee))

- Select the home directory when filtering for home ([#8](https://github.com/fullerzz/herdr-plugin-sesh/issues/8)) ([64bbb32](https://github.com/fullerzz/herdr-plugin-sesh/commit/64bbb32a41b9300b0043f315593ab8694840cb42))

- Misc improvements ([#7](https://github.com/fullerzz/herdr-plugin-sesh/issues/7)) ([e970aae](https://github.com/fullerzz/herdr-plugin-sesh/commit/e970aae91aab78b55e9c7ab323bf041d99db5a46))

- Scope startup commands to workspace ([#25](https://github.com/fullerzz/herdr-plugin-sesh/issues/25)) ([fe66957](https://github.com/fullerzz/herdr-plugin-sesh/commit/fe6695750535b07dddcb434ca28219b889427374))

- Align Sesh config runtime support ([#27](https://github.com/fullerzz/herdr-plugin-sesh/issues/27)) ([3927d67](https://github.com/fullerzz/herdr-plugin-sesh/commit/3927d67705303020ac86901dae94f89b2a50edd1))



### 💼 Other

- Merge pull request 'feat: Workspace Picker' ([#1](https://github.com/fullerzz/herdr-plugin-sesh/issues/1)) from picker into main ([1496b32](https://github.com/fullerzz/herdr-plugin-sesh/commit/1496b324c7f3ea0d22e045f5e1d3a5cc02056da6))

- Merge pull request 'feat: fzf and bat integration' ([#2](https://github.com/fullerzz/herdr-plugin-sesh/issues/2)) from fzf-integration into main ([3e97724](https://github.com/fullerzz/herdr-plugin-sesh/commit/3e977247b53cacdc82c8f07e00a99ebd3ffb5ff5))

- Merge pull request 'Misc Improvements and Bug Fixes for Picker' ([#3](https://github.com/fullerzz/herdr-plugin-sesh/issues/3)) from dev into main ([f0c0dbf](https://github.com/fullerzz/herdr-plugin-sesh/commit/f0c0dbfb69be2ebcf3bb9cffc5039039ad04cd8f))



### 🚜 Refactor

- Use GitHub module namespace ([#1](https://github.com/fullerzz/herdr-plugin-sesh/issues/1)) ([7991f50](https://github.com/fullerzz/herdr-plugin-sesh/commit/7991f50f7f629cf3745831f9ddab0fe0a2654174))



### 📚 Documentation

- Add AGENTS.md ([681941b](https://github.com/fullerzz/herdr-plugin-sesh/commit/681941bfc966c0b0e675acc5e102a5da415e698e))

- Document herdr plugin setup flow ([a758b9f](https://github.com/fullerzz/herdr-plugin-sesh/commit/a758b9febf0fb6db1e5cec0a332469f3fbc97e1b))

- Update AGENTS.md ([4e5d288](https://github.com/fullerzz/herdr-plugin-sesh/commit/4e5d288b828cbec9688f8fdce899477fa52c6d50))

- Update stale doc entries ([1a2235e](https://github.com/fullerzz/herdr-plugin-sesh/commit/1a2235ec519fe71207b7a6feec62582644535e65))

- Credit sesh ([3d30c9c](https://github.com/fullerzz/herdr-plugin-sesh/commit/3d30c9c4ea106e59df790965e92bc74f0d8efae6))



### 🎨 Styling

- Apply formatting ([a9d4583](https://github.com/fullerzz/herdr-plugin-sesh/commit/a9d4583434abed1c8e56b60b2ef5c59c40987384))



### ⚙️ Miscellaneous Tasks

- Add project config files ([3876035](https://github.com/fullerzz/herdr-plugin-sesh/commit/38760352f7d2bedaa62567ca1782d07edc27c6ad))

- Refresh x/sys dependency ([#18](https://github.com/fullerzz/herdr-plugin-sesh/issues/18)) ([fc061ad](https://github.com/fullerzz/herdr-plugin-sesh/commit/fc061ade28dc6fd059a300744ac66ee4ae473cc3))

- Pin mise tool versions ([#21](https://github.com/fullerzz/herdr-plugin-sesh/issues/21)) ([93aaacf](https://github.com/fullerzz/herdr-plugin-sesh/commit/93aaacfcf2b68c65ba56d10327ecd4e7c1a66772))



### 🎡 Continuous Integration

- Add forgejo test and release workflows ([98f6ea1](https://github.com/fullerzz/herdr-plugin-sesh/commit/98f6ea12e6860668c4367399f4871a3a1ae764d0))

- Run repo-native checks in GitHub Actions ([#19](https://github.com/fullerzz/herdr-plugin-sesh/issues/19)) ([121ae01](https://github.com/fullerzz/herdr-plugin-sesh/commit/121ae01febe4fe3361e77b3327f3773044e9d6b5))

- Check out requested release tag ([#16](https://github.com/fullerzz/herdr-plugin-sesh/issues/16)) ([2cddfbd](https://github.com/fullerzz/herdr-plugin-sesh/commit/2cddfbd34369ffb4aa0aebe81dba015f9556fbe7))

- Bind releases to requested tag commit ([#26](https://github.com/fullerzz/herdr-plugin-sesh/issues/26)) ([d1c43c1](https://github.com/fullerzz/herdr-plugin-sesh/commit/d1c43c1b99615e2bc64ad971916e6e44cdf9e802))

- Add git-cliff changelog generation to release workflow ([#28](https://github.com/fullerzz/herdr-plugin-sesh/issues/28)) ([20c118e](https://github.com/fullerzz/herdr-plugin-sesh/commit/20c118efb185164ff6137009701bb9709739f291))


