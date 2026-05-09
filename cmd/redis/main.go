// ------------------------------------------------------------------------------
// Copyright IBM Corp. 2018
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//    http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
// ------------------------------------------------------------------------------

package main

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/gomodule/redigo/redis"
	"github.com/mattn/go-isatty"
	"github.com/mattn/go-shellwords"
	"github.com/peterh/liner"
	"github.com/spf13/cobra"

	rediscommands "github.com/aaronjheng/redis-cli/internal/redis"
	"github.com/aaronjheng/redis-cli/internal/ssh"
)

const defaultRedisPort = 6379

var ( //nolint:gochecknoglobals,nolintlint
	redisurlStr     string   //nolint:gochecknoglobals
	redishost       string   //nolint:gochecknoglobals
	redisport       int      //nolint:gochecknoglobals
	user            string   //nolint:gochecknoglobals
	redisauth       string   //nolint:gochecknoglobals
	redisdb         int      //nolint:gochecknoglobals
	redistls        bool     //nolint:gochecknoglobals
	servername      string   //nolint:gochecknoglobals
	insecure        bool     //nolint:gochecknoglobals
	cacertfile      string   //nolint:gochecknoglobals
	rediscertb64    string   //nolint:gochecknoglobals
	forceraw        bool     //nolint:gochecknoglobals
	evalFile        string   //nolint:gochecknoglobals
	commandargs     []string //nolint:gochecknoglobals
	sshURI          string   //nolint:gochecknoglobals
	sshIdentityFile string   //nolint:gochecknoglobals
)

var ( //nolint:gochecknoglobals,nolintlint
	rawrediscommands = rediscommands.Commands{} //nolint:gochecknoglobals
	raw              = false                    //nolint:gochecknoglobals
)

var errCertLoadFailed = errors.New("couldn't load cert data")

func rootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:          "redis",
		Short:        "A Redis CLI client",
		SilenceUsage: true,
		Args:         cobra.ArbitraryArgs,
		RunE:         runE,
	}

	cmd.SetHelpCommand(&cobra.Command{Hidden: true})

	cmd.Flags().StringVarP(&redisurlStr, "uri", "u", "",
		"URI to connect to")
	cmd.Flags().StringVarP(&redishost, "host", "H", "127.0.0.1",
		"Host to connect to")
	cmd.Flags().IntVarP(&redisport, "port", "p", defaultRedisPort,
		"Port to connect to")
	cmd.Flags().StringVarP(&user, "user", "r", "",
		"Username to use when connecting. Supported since Redis 6.")
	cmd.Flags().StringVarP(&redisauth, "auth", "a", "",
		"Password to use when connecting")
	cmd.Flags().IntVarP(&redisdb, "db", "n", 0,
		"Redis database to access")
	cmd.Flags().BoolVar(&redistls, "tls", false,
		"Enable TLS/SSL")
	cmd.Flags().StringVarP(&servername, "sni", "s", "",
		"Server Name Indication for TLS certificate verification")
	cmd.Flags().BoolVar(&insecure, "insecure", false,
		"Disable certificate validation")
	cmd.Flags().StringVar(&cacertfile, "cacert", "",
		"CA certificate file for validation")
	cmd.Flags().StringVar(&rediscertb64, "certb64", "",
		"Self-signed certificate string as base64 for validation")
	cmd.Flags().BoolVar(&forceraw, "raw", false,
		"Produce raw output")
	cmd.Flags().StringVar(&evalFile, "eval", "",
		"Evaluate a Lua script file, follow with keys a , and args")
	cmd.Flags().StringVar(&sshURI, "ssh", "",
		"SSH tunnel connection URI. Format: [user[:pass]@]host[:port]")
	cmd.Flags().StringVar(&sshIdentityFile, "ssh-identity-file", "",
		"SSH identity file")

	cmd.Version = version
	cmd.InitDefaultVersionFlag()
	cmd.Flags().Lookup("version").Usage = "Print version"

	return cmd
}

func main() {
	err := rootCmd().Execute()
	if err != nil {
		os.Exit(1)
	}
}

func runE(cmd *cobra.Command, args []string) error {
	commandargs = args

	loadCertFromEnv(cmd)

	if forceraw {
		raw = true
	} else if !isatty.IsTerminal(os.Stdout.Fd()) && !isatty.IsCygwinTerminal(os.Stdout.Fd()) {
		raw = true
	}

	cert, err := loadCert()
	if err != nil {
		return err
	}

	connectionurl := buildConnectionURL()

	dialOptions, err := buildDialOptions(cert)
	if err != nil {
		return err
	}

	conn, err := dialRedis(connectionurl, dialOptions)
	if err != nil {
		return fmt.Errorf("dial: %w", err)
	}

	defer conn.Close()

	if evalFile != "" {
		return runEvalScript(conn)
	}

	if len(commandargs) > 0 {
		return runCommand(conn)
	}

	return runInteractive(conn)
}

func loadCertFromEnv(cmd *cobra.Command) {
	if !cmd.Flags().Changed("cacert") {
		if val := os.Getenv("REDIS_CACERT"); val != "" {
			cacertfile = val
		}
	}

	if !cmd.Flags().Changed("certb64") {
		if val := os.Getenv("REDIS_CERTB64"); val != "" {
			rediscertb64 = val
		}
	}
}

func loadCert() ([]byte, error) {
	if cacertfile != "" {
		cert, err := os.ReadFile(cacertfile)
		if err != nil {
			return nil, fmt.Errorf("read cert file: %w", err)
		}

		return cert, nil
	} else if rediscertb64 != "" {
		cert, err := base64.StdEncoding.DecodeString(rediscertb64)
		if err != nil {
			return nil, fmt.Errorf("decode base64 cert: %w", err)
		}

		return cert, nil
	}

	return nil, nil
}

func buildConnectionURL() string {
	var connectionurl string

	if redisurlStr == "" {
		if redistls {
			connectionurl = "rediss://"
		} else {
			connectionurl = "redis://"
		}

		if redisauth != "" {
			connectionurl += url.QueryEscape(user) + ":" + url.QueryEscape(redisauth) + "@"
		}

		connectionurl += redishost + ":" + strconv.Itoa(redisport) + "/" + strconv.Itoa(redisdb)
	} else {
		connectionurl = redisurlStr
	}

	return connectionurl
}

func buildDialOptions(cert []byte) ([]redis.DialOption, error) {
	dialOptions := []redis.DialOption{}

	//nolint:gosec
	config := &tls.Config{InsecureSkipVerify: insecure}

	if len(cert) > 0 {
		config.RootCAs = x509.NewCertPool()
		config.ClientAuth = tls.RequireAndVerifyClientCert

		ok := config.RootCAs.AppendCertsFromPEM(cert)
		if !ok {
			return nil, errCertLoadFailed
		}
	}

	if servername != "" {
		config.ServerName = servername
	}

	dialOptions = append(dialOptions, redis.DialTLSConfig(config))

	if sshURI != "" {
		sshDialOpts, err := buildSSHDialOptions()
		if err != nil {
			return nil, err
		}

		dialOptions = append(dialOptions, sshDialOpts...)
	}

	return dialOptions, nil
}

func buildSSHDialOptions() ([]redis.DialOption, error) {
	sshURL, err := url.Parse("ssh://" + sshURI)
	if err != nil {
		return nil, fmt.Errorf("parse SSH URI: %w", err)
	}

	password, _ := sshURL.User.Password()

	cfg := &ssh.Config{
		Host:         sshURL.Hostname(),
		Port:         sshURL.Port(),
		Username:     sshURL.User.Username(),
		Password:     password,
		IdentityFile: sshIdentityFile,
	}

	dialFunc, err := ssh.NewDialerFunc(cfg)
	if err != nil {
		return nil, fmt.Errorf("create SSH dialer: %w", err)
	}

	return []redis.DialOption{
		redis.DialContextFunc(dialFunc),
		redis.DialReadTimeout(0),
		redis.DialWriteTimeout(0),
	}, nil
}

func dialRedis(connectionurl string, dialOptions []redis.DialOption) (redis.Conn, error) {
	conn, err := redis.DialURL(connectionurl, dialOptions...)
	if err != nil && err.Error() == "ERR wrong number of arguments for 'auth' command" {
		re := regexp.MustCompile(`^(rediss?://)(.*)(:.*@.*)`)
		connectionurl = re.ReplaceAllString(connectionurl, `$1$3`)
		conn, err = redis.DialURL(connectionurl, dialOptions...)
	}

	if err != nil {
		return nil, fmt.Errorf("dial URL: %w", err)
	}

	return conn, nil
}

func runEvalScript(conn redis.Conn) error {
	scriptsrc, err := os.ReadFile(evalFile)
	if err != nil {
		return fmt.Errorf("read eval file: %w", err)
	}

	iargs, keycnt := parseEvalArgs()

	script := redis.NewScript(keycnt, string(scriptsrc))

	result, err := script.Do(conn, iargs...)
	if err != nil {
		return fmt.Errorf("eval script: %w", err)
	}

	printRedisResult(result, false)

	return nil
}

func parseEvalArgs() ([]any, int) {
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

func runCommand(conn redis.Conn) error {
	catchMonitorCmd(conn, commandargs[0])

	args := make([]any, len(commandargs[1:]))
	for idx, data := range commandargs[1:] {
		args[idx] = data
	}

	result, err := conn.Do(commandargs[0], args...)
	if err != nil {
		return fmt.Errorf("execute command: %w", err)
	}

	cmdForceRaw := strings.ToLower(commandargs[0]) == "info"

	printRedisResult(result, cmdForceRaw)

	return nil
}

func runInteractive(conn redis.Conn) error {
	err := json.Unmarshal(rediscommands.CommandsJSON, &rawrediscommands)
	if err != nil {
		return fmt.Errorf("unmarshal commands: %w", err)
	}

	rediscommandsMap, commandstrings := buildCommandIndex()

	linerInstance := liner.NewLiner()
	defer linerInstance.Close()

	linerInstance.SetCtrlCAborts(true)

	linerInstance.SetCompleter(func(line string) []string {
		return completeCommand(line, commandstrings)
	})

	interactiveLoop(conn, linerInstance, rediscommandsMap)

	return nil
}

func buildCommandIndex() (map[string]rediscommands.Command, []string) {
	rediscommandsMap := make(map[string]rediscommands.Command, len(rawrediscommands))
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

func completeCommand(line string, commandstrings []string) []string {
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

func interactiveLoop(conn redis.Conn, linerInstance *liner.State, rediscommandsMap map[string]rediscommands.Command) {
	for {
		cmdForceRaw := false

		line, err := linerInstance.Prompt(getPrompt())
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

		executeInteractiveCommand(conn, parts, cmdForceRaw)
	}
}

func executeInteractiveCommand(conn redis.Conn, parts []string, forceraw bool) {
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

	printRedisResult(result, forceraw)
}

//nolint:mnd
func handleHelpCommand(parts []string, rediscommandsMap map[string]rediscommands.Command) bool {
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

func printRedisResult(result any, forceraw bool) {
	printRedisResultIndenting(result, "", forceraw)
}

func printRedisResultIndenting(result any, prefix string, forceraw bool) {
	switch val := result.(type) {
	case []any:
		if isRawMode(forceraw) {
			printRedisResultListRaw(val, forceraw)
		} else {
			printRedisResultListFormatted(val, prefix, forceraw)
		}
	default:
		fmt.Fprintf(os.Stdout, "%s\n", toRedisValueString(result, forceraw))
	}
}

func printRedisResultListRaw(items []any, forceraw bool) {
	for _, item := range items {
		switch subItem := item.(type) {
		case []any:
			printRedisResultIndenting(subItem, "", forceraw)
		default:
			fmt.Fprintf(os.Stdout, "%s\n", toRedisValueString(subItem, forceraw))
		}
	}
}

func printRedisResultListFormatted(items []any, prefix string, forceraw bool) {
	spacer := strings.Repeat(" ", len(prefix))

	for idx, item := range items {
		switch subItem := item.(type) {
		case []any:
			newprefix := prefix + " " + strconv.FormatInt(int64(idx+1), 10) + ")"
			printRedisResultIndenting(subItem, newprefix, forceraw)
		default:
			if idx == 0 {
				fmt.Fprintf(os.Stdout, "%s %d) %s\n", prefix, idx+1, toRedisValueString(item, forceraw))
			} else {
				fmt.Fprintf(os.Stdout, "%s %d) %s\n", spacer, idx+1, toRedisValueString(item, forceraw))
			}
		}
	}
}

func isRawMode(forceraw bool) bool {
	return raw || forceraw
}

func toRedisValueString(value any, forceraw bool) string {
	switch val := value.(type) {
	case redis.Error:
		if isRawMode(forceraw) {
			return val.Error()
		}

		return "(error) " + val.Error()
	case int64:
		if isRawMode(forceraw) {
			return strconv.FormatInt(val, 10)
		}

		return "(integer) " + strconv.FormatInt(val, 10)
	case string:
		return val
	case []byte:
		if isRawMode(forceraw) {
			return string(val)
		}

		return "\"" + string(val) + "\""
	case nil:
		return "nil"
	}

	return ""
}

func getPrompt() string {
	if redisurlStr != "" {
		parsedURL, err := url.Parse(redisurlStr)
		if err == nil {
			return fmt.Sprintf("%s:%s> ", parsedURL.Hostname(), parsedURL.Port())
		}
	}

	return fmt.Sprintf("%s:%d> ", redishost, redisport)
}
