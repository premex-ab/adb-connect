# Release setup — secrets required

Before publishing the first release (pushing a `v*` tag), confirm these secrets are in place in GitHub Settings → Secrets and variables → Actions. The `PREMEX_BOT_APP_ID` and `PREMEX_BOT_APP_PRIVATE_KEY` secrets are organisation-level and shared across all `premex-ab` repos — they should already be present.

## Cross-repo push (Homebrew tap)

The release workflow mints a short-lived GitHub App installation token at runtime to push the updated Homebrew formula to `premex-ab/homebrew-tap`. No personal access token is required.

The following two secrets must exist at organisation level (Settings → Secrets → Actions in the `premex-ab` organisation):

- `PREMEX_BOT_APP_ID` — the numeric App ID of the Premex Bot GitHub App.
- `PREMEX_BOT_APP_PRIVATE_KEY` — the raw PEM text of the App's private key (multi-line, paste as-is into the secret value).

The `actions/create-github-app-token@v1` step in the release workflow reads these secrets, calls the GitHub Apps API to exchange them for a short-lived installation token scoped to `premex-ab/homebrew-tap`, and passes that token to Goreleaser as `HOMEBREW_TAP_TOKEN`. The token expires after one hour and is never stored.

**Prerequisites:** the Premex Bot GitHub App must be installed on both `premex-ab/adb-connect` (for the workflow to call the API) and `premex-ab/homebrew-tap` (to receive the formula push). Install the App from the App's settings page under "Install App".

## Android keystore and signing secrets

These secrets must be set at repository level in `premex-ab/adb-connect`:

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

**Keep `release.keystore` in a safe place outside the repo.** If you lose it, you cannot ship updates to the existing app install base.

## Homebrew tap

The tap repo `premex-ab/homebrew-tap` must exist before the first release.
Create it empty on GitHub; Goreleaser will push the first formula file into it.
The Premex Bot App must be installed on that repo with `contents: write` permission.
