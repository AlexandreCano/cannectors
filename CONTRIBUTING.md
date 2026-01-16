# Contributing to Canectors Runtime

Thank you for your interest in contributing to Canectors Runtime!

## Getting Started

1. Fork the repository
2. Clone your fork: `git clone https://github.com/YOUR_USERNAME/canectors-runtime.git`
3. Create a feature branch: `git checkout -b feature/your-feature-name`
4. Make your changes
5. Run tests: `make test`
6. Run linter: `make lint`
7. Commit your changes with a descriptive message
8. Push to your fork: `git push origin feature/your-feature-name`
9. Open a Pull Request

## Development Setup

### Requirements

- Go 1.23 or later
- Make (optional but recommended)
- golangci-lint (for linting)

### Building

```bash
make build
```

### Testing

```bash
# Run all tests
make test

# Run tests with coverage
make test-coverage

# Run tests with race detector
make test-race
```

### Code Quality

```bash
# Format code
make fmt

# Run linter
make lint

# Run all checks
make check
```

## Code Style

- Follow standard Go conventions
- Use `gofmt` for formatting
- Write tests for new functionality
- Add documentation comments for exported functions
- Keep functions focused and small

## Commit Messages

- Use clear, descriptive commit messages
- Start with a verb (Add, Fix, Update, Remove, etc.)
- Reference issues when applicable

## Pull Request Guidelines

1. Ensure all tests pass
2. Ensure linter passes with no errors
3. Update documentation if needed
4. Add tests for new functionality
5. Keep PRs focused on a single change

## Questions?

Open an issue for any questions or concerns.
