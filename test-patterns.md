# Test Patterns

## Mapping Fixtures

`internal/modules/filter/mapping_fixtures_test.go` centralizes mapping test setup:

- `rb()` builds `map[string]any` records without repeated inline literals.
- `mb()` builds `FieldMapping` values without repeated `strPtr` plumbing.
- `tb()` builds `TransformOp` values for transform-heavy cases.
- `assertMapped` and `assertRecordsEqual` keep process/assert boilerplate consistent.

Use these helpers when adding mapping tests before introducing new local setup.
