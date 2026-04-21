# Release setup — secrets required

Before publishing the first release (pushing a `v*` tag), configure these
repository secrets in GitHub Settings → Secrets and variables → Actions:

- `HOMEBREW_TAP_TOKEN` — fine-grained PAT with `contents: write` on
  `premex-ab/homebrew-tap`. Used by Goreleaser's `brews.repository.token`.
- `ANDROID_KEYSTORE_B64` — base64-encoded keystore file
  (`base64 -i release.keystore -o release.keystore.b64`).
- `ANDROID_KEYSTORE_PASSWORD` — keystore password.
- `ANDROID_KEY_ALIAS` — key alias inside the keystore (`adbgate`).
- `ANDROID_KEY_PASSWORD` — individual key password.

## Generating the keystore (one-time)

    keytool -genkey -v -keystore release.keystore \
      -alias adbgate -keyalg RSA -keysize 4096 -validity 10950 \
      -storepass <store-pass> -keypass <key-pass> \
      -dname "CN=Premex AB, O=Premex, C=SE"

Base64-encode it:

    base64 -i release.keystore -o release.keystore.b64

Copy the contents of `release.keystore.b64` into the `ANDROID_KEYSTORE_B64` secret.

**Keep `release.keystore` in a safe place outside the repo.** If you lose it,
you cannot ship updates to the existing app install base.

## Homebrew tap

The tap repo `premex-ab/homebrew-tap` must exist before the first release.
Create it empty on GitHub; Goreleaser will push the first formula file into it.
The `HOMEBREW_TAP_TOKEN` PAT must have write access to that repo.
