set dotenv-load

lint:
	golangci-lint run --verbose --allow-parallel-runners

