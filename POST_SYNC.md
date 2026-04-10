Fixes release tagging so the published tag always points to the commit that contains the updated version number.

## Fixes
- Fixes release workflow so the git tag is moved to the version-bump commit, ensuring the tag and `main` are always in sync after a release
