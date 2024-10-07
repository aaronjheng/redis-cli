set dotenv-load

lint:
	golangci-lint run --verbose --allow-parallel-runners

update-commands:
	curl -LO https://github.com/antirez/redis-doc/raw/master/commands.json

