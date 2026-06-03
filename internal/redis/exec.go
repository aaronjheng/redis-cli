package redis

import (
	"fmt"
	"os"
	"strings"

	"github.com/gomodule/redigo/redis"
)

func RunCommand(conn redis.Conn, args []string, printer *Printer) error {
	catchMonitorCmd(conn, args[0])

	cmdArgs := make([]any, len(args[1:]))
	for idx, data := range args[1:] {
		cmdArgs[idx] = data
	}

	result, err := conn.Do(args[0], cmdArgs...)
	if err != nil {
		return fmt.Errorf("execute command: %w", err)
	}

	cmdForceRaw := strings.ToLower(args[0]) == "info"
	printer.PrintIndenting(result, "", cmdForceRaw)

	return nil
}

func RunEvalScript(conn redis.Conn, scriptPath string, args []string, printer *Printer) error {
	scriptsrc, err := os.ReadFile(scriptPath)
	if err != nil {
		return fmt.Errorf("read eval file: %w", err)
	}

	iargs, keycnt := ParseEvalArgs(args)

	script := redis.NewScript(keycnt, string(scriptsrc))

	result, err := script.Do(conn, iargs...)
	if err != nil {
		return fmt.Errorf("eval script: %w", err)
	}

	printer.Print(result)

	return nil
}

func ParseEvalArgs(commandargs []string) ([]any, int) {
	if len(commandargs) == 0 {
		return nil, 0
	}

	args := make([]any, len(commandargs))
	keycnt := 0
	gotcomma := false

	for idx, data := range commandargs {
		if !gotcomma {
			if data == "," {
				gotcomma = true
			} else {
				args[idx] = data
				keycnt++
			}
		} else {
			args[idx-1] = data
		}
	}

	return args, keycnt
}

// catchMonitorCmd to go into a "stream" mode to stream back
// every command processed by Redis server.
func catchMonitorCmd(conn redis.Conn, command string) {
	if strings.ToLower(command) != "monitor" {
		return
	}

	_, err := conn.Do("monitor")
	if err != nil {
		return
	}

	for {
		line, err := redis.String(conn.Receive())
		if err != nil {
			return
		}

		fmt.Fprintf(os.Stdout, "%s\n", line)
	}
}
