set dotenv-load := true

bump-deps:
    go get -u ./...
    go mod tidy

lint:
    golangci-lint run --verbose --allow-parallel-runners

update-commands:
    curl -o cmd/redis/commands.json -LO https://github.com/antirez/redis-doc/raw/master/commands.json
