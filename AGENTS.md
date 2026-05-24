# Agent Rules

1. **Read before writing.** Always read relevant source files, documentation, and existing patterns in the codebase before making any changes.
2. **Verify dependency versions.** Cross-check any dependency APIs or behavior against the versions declared in `go.mod`. Current key dependencies include Gio UI `v0.10.0` and `golang.org/x/sys v0.45.0`; do not assume newer or older behavior.
