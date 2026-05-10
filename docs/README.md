# Cannectors Documentation

This directory contains the maintained project documentation. The root
`README.md` is intentionally short; use these pages for implementation details.

## User Documentation

- [Configuration format](CONFIGURATION.md) - pipeline shape, defaults,
  authentication, retry, and state persistence.
- [Module reference](MODULES.md) - supported input, filter, and output modules.
- [CLI reference](CLI.md) - commands, flags, exit codes, and examples.
- [Operations guide](OPERATIONS.md) - scheduling, state, logs, test lab, and
  release commands.
- [Examples index](../examples/README.md) - maintained pipeline templates.

## Developer Documentation

- [Architecture](ARCHITECTURE.md)
- [Module boundaries](MODULE_BOUNDARIES.md)
- [Module extensibility](MODULE_EXTENSIBILITY.md)
- [Execution flow](EXECUTION_FLOW.md)
- [Schema/runtime drift](SCHEMA_RUNTIME_DRIFT.md)
- [Package map](PACKAGE_MAP.md)
- [Type and method map](TYPE_AND_METHOD_MAP.md)

## Historical Notes

The files below are retained as project history or refactoring context. Treat
them as dated analysis, not as the current user-facing source of truth.

- [Technical audit 2026-04-21](AUDIT_TECHNIQUE_2026-04-21.md)
- [Cleanup plan](CLEANUP_PLAN.md)
- [Schema-driven audit plan](plan.md)
- [onError centralization plan](plan-centralisation-onerror.md)
- [Module config refactoring plan](plan-refactoring-module-config.md)
- [Refactoring notes](REFACTORING_NOTES.md)
- [Go concepts for Java developers](GO_CONCEPTS_FOR_JAVA_DEVS.md)
