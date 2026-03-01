# Contributing

Contributions are welcome. Here's how to help.

## Getting Started

```bash
git clone https://github.com/krishna-kudari/ratelimit.git
cd ratelimit
make test
```

## Development

```bash
make test       # run tests with race detector
make lint       # run golangci-lint
make bench      # run benchmarks
make fmt        # format code
```

## Pull Requests

1. Fork the repo and create a branch from `main`.
2. Add tests for new functionality.
3. Ensure `make ci` passes (vet, test, lint, bench).
4. Keep commits focused — one logical change per commit.
5. Write a clear PR description explaining **why**, not just what.

## Reporting Bugs

Open an issue using the **Bug Report** template. Include:
- Go version (`go version`)
- Steps to reproduce
- Expected vs actual behavior

## Code Style

- Follow standard Go conventions (`gofmt`, `go vet`).
- Exported functions require doc comments.
- No unnecessary dependencies.

## License

By contributing, you agree that your contributions will be licensed under the [MIT License](LICENSE).
