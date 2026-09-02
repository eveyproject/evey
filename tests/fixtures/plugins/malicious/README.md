# Malicious Plugin Fixtures

These fixtures exist solely for anti-regression testing of the static analyzer
(`internal/adapters/driven/pluginrunner/analyzer.go`). Each fixture contains a
real bypass payload. The test in `internal/app/plugin/installer_security_test.go`
runs `AnalyzePlugin` against every fixture and asserts that at least one
`Critical` finding is emitted.

If a future change to the analyzer accidentally removes a detection rule, the
corresponding test case turns red - forcing the author to either restore the
coverage or explicitly acknowledge the regression.

## Fixtures

| Directory | Language | Bypass technique |
|---|---|---|
| `php-string-concat-rce` | PHP | String concatenation hides `system()` from naive grep |
| `python-fstring-eval` | Python | f-string fragments reassemble `eval` at runtime |
| `js-constructor-escape` | JS/Node | `Function.constructor` reconstructs `execSync` call |
| `python-os-alias` | Python | Alias `os` to another name before calling `.system()` |
| `python-getattr-builtin` | Python | `getattr(__builtins__, ...)` retrieves `exec` dynamically |
| `go-raw-syscall` | Go | Direct `syscall.Syscall(59, ...)` = raw `execve` without stdlib |
| `php-combo-realistic` | PHP | Multi-vector: RCE + JWT exfil + outbound HTTP |
| `python-exec-base64` | Python | Payload is a base64 blob decoded into `exec()` - nothing dangerous is spelled out in the source |
| `js-datafile-function` | JS/Node | Payload lives in a `.dat` file and is turned into a function at runtime - loader and payload are innocuous apart |

## Adding a new fixture

1. Create a subdirectory under `tests/fixtures/plugins/malicious/<name>/`.
2. Add a minimal `plugin.yaml` (name, version, kind: plugin, language, entrypoint).
3. Add the source file with the bypass payload.
4. Add a test case in `internal/app/plugin/installer_security_test.go` asserting
   the expected `Critical` category.

## WARNING

Do NOT extract and execute these files. They contain real exploitation
payloads intended only for static analysis testing.
