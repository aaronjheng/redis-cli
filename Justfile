set dotenv-load

bump-deps:
    go get -u ./...
    go mod tidy

lint:
    golangci-lint run --verbose --allow-parallel-runners

lint-with-fix:
    golangci-lint run --verbose --allow-parallel-runners --fix ./...

update-commands:
    curl -o cmd/redis/commands.json -LO https://github.com/antirez/redis-doc/raw/master/commands.json

install:
    go install github.com/aaronjheng/redis-cli/cmd/redis@$(git ls-remote https://github.com/aaronjheng/redis-cli.git refs/heads/master | cut -f1)
