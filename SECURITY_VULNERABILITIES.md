# FyClip Security Review

**Review date:** 2026-10-06

**Scope:** Go application source, updater, local persistence, backup import, clipboard parsing, and UI preview paths.

**Method:** Manual source review and targeted static searches. No dynamic tests or external dependency audit were run.

## Current remediation status

The previous version of this document described 19 issues as fixed, but multiple descriptions did not match this checkout. Those claims have been removed. The findings below are based on the current source tree; “Fixed” means a source change was made during this review, not that it has passed an automated security test.

### Fixed in this review

1. **Update asset digest was never enforced — High (CWE-494).** `ExpectedHash` existed but was not populated from GitHub release metadata, and installation merely logged the local hash. The updater now reads GitHub's asset `digest`, rejects missing or malformed SHA-256 digests, verifies the download, and verifies it again immediately before execution. The command-line update path passes the expected digest to the installer. If a release omits a digest, automatic installation now fails closed.

2. **Update download path collision and symlink clobber — High (CWE-377).** Downloads used a predictable filename directly under the shared temporary directory and `os.Create`, which could overwrite an existing file. Each download now gets a private temporary directory and creates the asset with exclusive creation and owner-only permissions.

3. **Updater accepted arbitrary download URLs and redirects — High (CWE-601/SSRF).** A URL supplied by release metadata was fetched without host restrictions. The updater now requires the initial URL to be HTTPS on `github.com`, and after redirects accepts only HTTPS GitHub release and GitHub asset CDN hosts.

4. **Unbounded update and package extraction — High (CWE-400/CWE-409).** Release metadata, downloaded assets, ZIP expanded size, local history, backups, and snippets lacked consistent size limits. The updater now caps metadata at 2 MiB and downloaded/expanded data at 1 GiB. Local encrypted history, backup imports/info, decompression, and snippet input are capped at 256 MiB; history, backups, and snippets are limited to 100,000 entries.

5. **ZIP Slip and external `unzip` extraction — High (CWE-22).** macOS ZIP updates were extracted by invoking `unzip` into a shared predictable directory. Extraction now uses Go's ZIP reader in a private temporary directory, rejects symbolic links and paths escaping the extraction root, uses exclusive file creation, and enforces an expanded-size limit.

6. **Legacy encryption-key migration could invalidate stored history — High (CWE-311/data loss).** The asynchronous migration replaced the old key with unrelated PBKDF2 material without re-encrypting history. The migration was removed; existing legacy keys continue to be loaded unchanged. This fixes the destructive migration behavior, but does not upgrade legacy key storage.

7. **System command validation rejected commands outside temp — Medium (CWE-697).** `validateCommandExists` incorrectly required tools such as `dpkg`, `sudo`, and `msiexec` to be located under the temporary directory. It now checks that the resolved command exists and is a file.

8. **Backup schema validation was too weak — Medium (CWE-20).** Backup restore accepted any non-empty version/checksum and checked only item IDs and types. It now enforces the supported version, checksum shape, item-count and per-item validations before restoring. Backup inspection and import also use bounded reads.

9. **Regex cache could grow without limit — Medium (CWE-400).** Arbitrary search patterns were retained indefinitely. Patterns over 1,000 bytes bypass compilation, and the cache is reset when it reaches 128 entries.

10. **Weakly bounded clipboard item fields — Medium (CWE-20).** Restored items now pass `ValidateItem`, which checks type-specific input and enforces the content-size limit across all item types.

11. **Clipboard subprocess output buffered without a limit — Medium (CWE-400).** `xclip` and `wl-paste` output used `Cmd.Output()`, which can buffer arbitrary clipboard contents. Linux helper output now uses bounded writers; monitor handlers also reject text, image, and HTML values above their configured limits before conversion and storage.

12. **Image decompression bomb through clipboard history — High (CWE-409).** Thumbnail generation decoded arbitrary image dimensions before resizing, allowing a small compressed image to request a huge pixel buffer. Image decoding now checks dimensions and limits area to 16 million pixels before full decode; preview validates dimensions before handing data to Fyne. Invalid items are also rejected by `Manager.AddItem`, and `Storage.Save` validates records before writing.

## Remaining risks and limitations

1. **Encryption key is stored beside ciphertext — Medium (CWE-522).** `encryption.key` (or its entropy input and salt) is stored in the same user data directory as the encrypted clipboard history. File permissions protect against other local accounts when honored, but this design does not protect the data from malware or an attacker running as the same user. A platform keychain integration or a user-supplied secret is needed for that threat model.

2. **Legacy backup encryption uses raw SHA-256 — Medium (CWE-916).** New password-protected backups use PBKDF2, but the compatibility decryption fallback still accepts backups encrypted with `SHA256(password)` and no salt. These legacy files remain vulnerable to offline password guessing. Re-encrypt old backups after restoring them.

3. **GitHub asset digest trust depends on GitHub metadata/TLS.** SHA-256 detects a mismatch against the digest returned by the GitHub API, but it is not a publisher signature. Compromise of the GitHub release/account or API transport trust could replace both asset and digest. Signed release metadata or an independently trusted signing key would strengthen this.

4. **Windows/Linux/macOS installation paths need platform review.** This was a source-only review; platform-specific command execution, package-manager behavior, macOS mount parsing, and Windows installer behavior have not been exercised on their target operating systems.

5. **No automated verification or dependency vulnerability scan was run.** Review changes have not been validated with tests, cross-compilation, `govulncheck`, or a dependency audit.

6. **Native clipboard libraries may allocate before monitor limits apply — Low/Medium (CWE-400).** The in-process clipboard backend returns an already allocated byte slice, so the monitor can stop further processing but cannot cap allocation inside that dependency. The subprocess fallback does have an output cap.

## Files changed in this review

- `internal/update/checker.go`
- `internal/clipboard/storage.go`
- `internal/clipboard/backup.go`
- `internal/clipboard/snippet.go`
- `internal/clipboard/validation.go`
- `internal/clipboard/search.go`
- `internal/clipboard/native.go`
- `internal/clipboard/monitor.go`
- `internal/clipboard/manager.go`
- `internal/ui/preview.go`
- `internal/clipboard/item.go`
- `main.go`
