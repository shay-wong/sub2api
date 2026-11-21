# Integration test selection

Run `make test-unit` for the complete unit and shared test suite. Run `make test-integration` for the integration-only test entries, in parallel on a separate CI runner.

The integration runner compares `go list` results for the `unit` and `integration` build tags. It parses test entry points from files added by the integration build, including external test packages, fuzz seeds and runnable examples. File comparison preserves different implementations with the same name, such as the unit and integration TLS tests. Shared files run once in the unit suite. `TestMain` still initializes the selected package normally. The empty `TestUserRepoSortSuiteSmoke` and skip-only `TestDingTalkOAuthStart_Disabled` placeholders were removed; the actual sorting and DingTalk assertions remain.

The selected packages run in one `go test` command, preserving Go's package parallelism and failure exit status. A missing or invalid source file, a failed `go list`, or an empty selection fails the command. Future tests are discovered automatically. Shared tests whose behavior intentionally differs under the integration tag should be moved to an integration-specific file.

From `backend/`:

```sh
go run ./scripts/integration-tests --list
go run ./scripts/integration-tests -count=1 -json -timeout=10m
go test -tags=unit ./scripts/integration-tests
```

CI shares downloaded Go modules with the release workflow and uses separate build caches for unit tests, integration tests and lint. Only the unit job saves the shared module cache. The test matrix does not cancel the other suite on failure. The resulting GitHub check names are `test (unit)` and `test (integration)`.

## Measured validation

On the same macOS ARM64 machine with warm build caches and `-count=1`, the interval between the first and last Go test JSON events was:

| Integration command | Test execution time | Test entries |
| --- | ---: | ---: |
| `go test -json -tags=integration ./... -count=1 -timeout=8m` | 175.134 s | 8,178 |
| `go run ./scripts/integration-tests -json -count=1 -timeout=8m` | 19.364 s | 286 |

The 286 selected entries and all their subtests matched the baseline execution (984 results, including two existing skips). The baseline also included shared tests that still run in the unit suite and the two removed placeholders. These timings exclude dependency downloads and initial compilation; the approximately 89% reduction applies to local integration test execution, not the entire GitHub Actions workflow.
