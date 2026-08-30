# Maintainers

| Maintainer          | GitHub     | Areas                          |
| ------------------- | ---------- | ------------------------------ |
| Christopher Junius  | @cjunius   | everything (search, eval, UCI) |

## Responsibilities

- Triage issues and review pull requests within a reasonable time.
- Keep CI green on `main`.
- Cut releases (see below).
- Respond to security reports per [SECURITY.md](SECURITY.md).

## Release process

1. Ensure `main` is green and `CHANGELOG.md` `## [Unreleased]` is complete.
2. Move the `## [Unreleased]` entries under a new `## [vX.Y.Z] - YYYY-MM-DD`
   heading and add the compare link.
3. Tag: `git tag -s vX.Y.Z -m "vX.Y.Z" && git push origin vX.Y.Z`.
4. The `release` workflow runs GoReleaser, which builds cross-platform binaries,
   generates checksums and an SBOM, and creates the GitHub Release.
5. Verify the release artifacts and announce in Discussions.

## Adding a maintainer

Open a PR editing this file. Requires sign-off from all current maintainers.
