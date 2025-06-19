module github.com/aaronjheng/redis-cli

go 1.24.0

replace github.com/gomodule/redigo => ./third_party/redigo

require (
	github.com/alecthomas/kingpin/v2 v2.4.0
	github.com/gomodule/redigo v1.9.2
	github.com/mattn/go-isatty v0.0.20
	github.com/mattn/go-shellwords v1.0.12
	github.com/peterh/liner v1.2.2
	golang.org/x/crypto v0.32.0
)

require (
	github.com/alecthomas/units v0.0.0-20240927000941-0f3dac36c52b // indirect
	github.com/mattn/go-runewidth v0.0.16 // indirect
	github.com/rivo/uniseg v0.4.7 // indirect
	github.com/xhit/go-str2duration/v2 v2.1.0 // indirect
	golang.org/x/sys v0.29.0 // indirect
)
