# AGENTS.md

We are porting a legacy nim codebase to D. 
the legacy codebase is in the legacy dir -> do not touch this. Only read it for references purposes. 



## Build, Lint, and Test Commands
- **Build:** `dub build` (from project root)
- **Run:** `dub run` (from project root)
- **Format:** `dfmt -i src/` (requires .dfmt.toml)
- **Test:** `dub test` (no test files found; add tests in `tests/` or as `unittest` blocks)
- **Single Test:** DUB does not natively support single test runs; use selective `unittest` blocks and filter output.

## Code Style Guidelines
- **Imports:** Group at top; use selective imports (e.g., `import std.string : startsWith`).
- **Formatting:** Use `dfmt` for consistent style.
- **Types:** Prefer `struct` for option groups, `enum` for constants.
- **Naming:**
  - Types: `CamelCase`
  - Variables/Functions: `lowerCamelCase`
- **Error Handling:**
  - Return error codes (0 for success, 2 for unknown command).
  - Print errors to stderr using UI helpers.
- **Command Structure:**
  - Main entrypoint delegates to `run(args)`.
  - Use option structs for command arguments.
- **Comments:** Use `//` for single-line, place above relevant code.
* Keep things simple and not too complex

## Additional Notes
- No Cursor or Copilot rules found.
- Add tests in `tests/` or as `unittest` blocks for coverage.
