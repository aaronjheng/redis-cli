package redis

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"sort"
	"strings"

	redigo "github.com/gomodule/redigo/redis"
	"github.com/mattn/go-shellwords"
	"github.com/peterh/liner"
)

func RunInteractive(conn redigo.Conn, redisurlStr, redishost string, redisport int, printer *Printer) error {
	var rawrediscommands Commands

	err := json.Unmarshal(CommandsJSON, &rawrediscommands)
	if err != nil {
		return fmt.Errorf("unmarshal commands: %w", err)
	}

	rediscommandsMap, commandstrings := buildCommandIndex(rawrediscommands)

	linerInstance := liner.NewLiner()
	defer linerInstance.Close()

	linerInstance.SetCtrlCAborts(true)
	linerInstance.SetCompleter(func(line string) []string {
		return CompleteCommand(line, commandstrings)
	})

	interactiveLoop(conn, linerInstance, rediscommandsMap, printer, redisurlStr, redishost, redisport)

	return nil
}

func buildCommandIndex(rawrediscommands Commands) (map[string]Command, []string) {
	rediscommandsMap := make(map[string]Command, len(rawrediscommands))
	commandstrings := make([]string, len(rawrediscommands))

	idx := 0

	for key, val := range rawrediscommands {
		command := strings.ToLower(key)
		commandstrings[idx] = command
		idx++
		rediscommandsMap[command] = val
	}

	sort.Strings(commandstrings)

	return rediscommandsMap, commandstrings
}

func CompleteCommand(line string, commandstrings []string) []string {
	var completions []string

	lowerline := strings.ToLower(line)
	for _, name := range commandstrings {
		if strings.HasPrefix(name, lowerline) {
			completions = append(completions, name)
		}
	}

	if len(completions) == 0 {
		if after, ok := strings.CutPrefix(lowerline, "help "); ok {
			for _, name := range commandstrings {
				if strings.HasPrefix(name, after) {
					completions = append(completions, "help "+name)
				}
			}
		}
	}

	return completions
}

func interactiveLoop(
	conn redigo.Conn,
	linerInstance *liner.State,
	rediscommandsMap map[string]Command,
	printer *Printer,
	redisurlStr, redishost string,
	redisport int,
) {
	for {
		cmdForceRaw := false

		line, err := linerInstance.Prompt(getPrompt(redisurlStr, redishost, redisport))
		if err != nil {
			return
		}

		if len(line) == 0 {
			continue
		}

		parts, err := shellwords.Parse(line)
		if err != nil {
			continue
		}

		if len(parts) == 0 {
			continue
		}

		linerInstance.AppendHistory(line)

		if parts[0] == "help" {
			if handleHelpCommand(parts, rediscommandsMap) {
				continue
			}
		}

		if parts[0] == "exit" {
			return
		}

		if strings.ToLower(parts[0]) == "info" {
			cmdForceRaw = true
		}

		executeInteractiveCommand(conn, parts, printer, cmdForceRaw)
	}
}

func executeInteractiveCommand(conn redigo.Conn, parts []string, printer *Printer, forceRaw bool) {
	args := make([]any, len(parts[1:]))
	for idx, data := range parts[1:] {
		args[idx] = data
	}

	catchMonitorCmd(conn, parts[0])

	result, err := conn.Do(parts[0], args...)
	if err != nil {
		fmt.Fprintf(os.Stderr, "(error) %s\n", err)

		return
	}

	printer.PrintIndenting(result, "", forceRaw)
}

//nolint:mnd
func handleHelpCommand(parts []string, rediscommandsMap map[string]Command) bool {
	if len(parts) == 1 {
		fmt.Fprintln(os.Stdout, "Enter help <command> to show information about a command")

		return true
	}

	lookup := parts[1]
	if len(parts) == 3 {
		lookup = parts[1] + " " + parts[2]
	}

	commanddata, ok := rediscommandsMap[lookup]
	if !ok {
		return false
	}

	fmt.Fprintf(os.Stdout, "Command: %s\n", strings.ToUpper(lookup))
	fmt.Fprintf(os.Stdout, "Summary: %s\n", commanddata.Summary)

	if commanddata.Complexity != "" {
		fmt.Fprintf(os.Stdout, "Complexity: %s\n", commanddata.Complexity)
	}

	if commanddata.Arguments != nil {
		fmt.Fprintln(os.Stdout, "Args:")

		for _, arg := range commanddata.Arguments {
			fmt.Fprintf(os.Stdout, "     %s (%s)\n", arg.Name, arg.Type)
		}
	}

	return true
}

func getPrompt(redisurlStr, redishost string, redisport int) string {
	if redisurlStr != "" {
		parsedURL, err := url.Parse(redisurlStr)
		if err == nil {
			return fmt.Sprintf("%s:%s> ", parsedURL.Hostname(), parsedURL.Port())
		}
	}

	return fmt.Sprintf("%s:%d> ", redishost, redisport)
}
