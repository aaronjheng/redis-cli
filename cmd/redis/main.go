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
	_ "embed"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/aaronjheng/redis-cli/internal/ssh"
	"github.com/gomodule/redigo/redis"
	"github.com/mattn/go-isatty"
	"github.com/mattn/go-shellwords"
	"github.com/peterh/liner"
	"github.com/spf13/cobra"
)

var (
	redisurlStr   string
	redishost     string
	redisport     int
	redisuser     string
	redisauth     string
	redisdb       int
	redistls      bool
	servername    string
	skipverify    bool
	rediscertfile string
	rediscertb64  string
	forceraw      bool
	evalFile      string
	commandargs   []string

	sshUri          string
	sshIdentityFile string
)

var (
	rawrediscommands = Commands{}
	raw              = false
)

//go:embed commands.json
var redisCommandsJSON []byte

var rootCmd = &cobra.Command{
	Use:     "redis-cli",
	Short:   "A Redis CLI client",
	Version: version,
	Run:     run,
}

func init() {
	rootCmd.Flags().StringVarP(&redisurlStr, "uri", "u", "", "URI to connect to")
	rootCmd.Flags().StringVarP(&redishost, "host", "H", "127.0.0.1", "Host to connect to")
	rootCmd.Flags().IntVarP(&redisport, "port", "p", 6379, "Port to connect to")
	rootCmd.Flags().StringVarP(&redisuser, "redisuser", "r", "", "Username to use when connecting. Supported since Redis 6.")
	rootCmd.Flags().StringVarP(&redisauth, "auth", "a", "", "Password to use when connecting")
	rootCmd.Flags().IntVarP(&redisdb, "ndb", "n", 0, "Redis database to access")
	rootCmd.Flags().BoolVar(&redistls, "tls", false, "Enable TLS/SSL")
	rootCmd.Flags().StringVarP(&servername, "servername", "s", "", "ServerName is used to verify the hostname on the returned certificates unless skipverify is set.")
	rootCmd.Flags().BoolVar(&skipverify, "skipverify", false, "Don't validate certificates")
	rootCmd.Flags().StringVar(&rediscertfile, "certfile", "", "Self-signed certificate file for validation")
	rootCmd.Flags().StringVar(&rediscertb64, "certb64", "", "Self-signed certificate string as base64 for validation")
	rootCmd.Flags().BoolVar(&forceraw, "raw", false, "Produce raw output")
	rootCmd.Flags().StringVar(&evalFile, "eval", "", "Evaluate a Lua script file, follow with keys a , and args")
	rootCmd.Flags().StringVar(&sshUri, "ssh", "", "SSH tunnel connection URI. Format: [user[:pass]@]host[:port]")
	rootCmd.Flags().StringVar(&sshIdentityFile, "ssh-identity-file", "", "SSH identity file")

}

func main() {
	rootCmd.Args = cobra.ArbitraryArgs
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func run(cmd *cobra.Command, args []string) {
	commandargs = args

	if !cmd.Flags().Changed("certfile") {
		if v := os.Getenv("REDIS_CERTFILE"); v != "" {
			rediscertfile = v
		}
	}
	if !cmd.Flags().Changed("certb64") {
		if v := os.Getenv("REDIS_CERTB64"); v != "" {
			rediscertb64 = v
		}
	}

	if forceraw {
		raw = true
	} else {
		if !isatty.IsTerminal(os.Stdout.Fd()) && !isatty.IsCygwinTerminal(os.Stdout.Fd()) {
			raw = true
		}
	}

	cert := []byte{}

	if rediscertfile != "" {
		mycert, err := os.ReadFile(rediscertfile)
		if err != nil {
			log.Fatal(err)
		}
		cert = mycert
	} else if rediscertb64 != "" {
		mycert, err := base64.StdEncoding.DecodeString(rediscertb64)
		if err != nil {
			log.Fatal("What", err)
		}
		cert = mycert
	}

	connectionurl := ""

	if redisurlStr == "" {
		if redistls {
			connectionurl = "rediss://"
		} else {
			connectionurl = "redis://"
		}

		if redisauth != "" {
			connectionurl = connectionurl + url.QueryEscape(redisuser) + ":" + url.QueryEscape(redisauth) + "@"
		}

		connectionurl = connectionurl + redishost + ":" + strconv.Itoa(redisport) + "/" + strconv.Itoa(redisdb)
	} else {
		connectionurl = redisurlStr
	}

	dialOptions := []redis.DialOption{}

	config := &tls.Config{InsecureSkipVerify: skipverify}
	if len(cert) > 0 {
		config.RootCAs = x509.NewCertPool()
		config.ClientAuth = tls.RequireAndVerifyClientCert
		ok := config.RootCAs.AppendCertsFromPEM(cert)
		if !ok {
			log.Fatal("Couldn't load cert data")
		}
	}

	if servername != "" {
		config.ServerName = servername
	}

	dialOptions = append(dialOptions, redis.DialTLSConfig(config))

	if sshUri != "" {
		u, err := url.Parse("ssh://" + sshUri)
		if err != nil {
		}

		password, _ := u.User.Password()

		cfg := &ssh.Config{
			Host:         u.Hostname(),
			Port:         u.Port(),
			Username:     u.User.Username(),
			Password:     password,
			IdentityFile: sshIdentityFile,
		}
		dialFunc, err := ssh.NewDialerFunc(cfg)
		if err != nil {
		}

		dialOptions = append(dialOptions,
			redis.DialContextFunc(dialFunc),
			redis.DialReadTimeout(0),
			redis.DialWriteTimeout(0),
		)
	}

	conn, err := redis.DialURL(connectionurl, dialOptions...)

	if err != nil && err.Error() == "ERR wrong number of arguments for 'auth' command" {
		re := regexp.MustCompile(`^(rediss?://)(.*)(:.*@.*)`)
		connectionurl = re.ReplaceAllString(connectionurl, `$1$3`)
		conn, err = redis.DialURL(connectionurl, dialOptions...)
	}

	if err != nil {
		log.Fatal("Dial ", err)
	}
	defer conn.Close()

	if evalFile != "" {
		scriptsrc, err := os.ReadFile(evalFile)
		if err != nil {
			log.Fatal(err)
		}

		var iargs []interface{}

		keycnt := 0

		if len(commandargs) > 0 {
			args := make([]interface{}, len(commandargs[:]))

			gotcomma := false

			for i, d := range commandargs {
				if !gotcomma {
					if d == "," {
						gotcomma = true
					} else {
						args[i] = d
						keycnt = keycnt + 1
					}
				} else {
					args[i-1] = d
				}
			}

			iargs = append(iargs, args...)
		}

		script := redis.NewScript(keycnt, string(scriptsrc[:]))
		result, err := script.Do(conn, iargs...)
		if err != nil {
			log.Fatal(err)
		}

		printRedisResult(result, false)

		os.Exit(0)
	}

	if len(commandargs) > 0 {
		catchMonitorCmd(conn, commandargs[0])

		args := make([]interface{}, len(commandargs[1:]))
		for i, d := range commandargs[1:] {
			args[i] = d
		}
		result, err := conn.Do(commandargs[0], args...)
		if err != nil {
			log.Fatal(err)
		}

		forceraw := false

		if strings.ToLower(commandargs[0]) == "info" {
			forceraw = true
		}

		printRedisResult(result, forceraw)

		os.Exit(0)
	}

	json.Unmarshal([]byte(redisCommandsJSON), &rawrediscommands)

	rediscommands := make(map[string]Command, len(rawrediscommands))
	commandstrings := make([]string, len(rawrediscommands))

	i := 0
	for k, v := range rawrediscommands {
		command := strings.ToLower(k)
		commandstrings[i] = command
		i = i + 1
		rediscommands[command] = v
	}

	sort.Strings(commandstrings)

	liner := liner.NewLiner()
	defer liner.Close()

	liner.SetCtrlCAborts(true)

	liner.SetCompleter(func(line string) (c []string) {
		lowerline := strings.ToLower(line)
		for _, n := range commandstrings {
			if strings.HasPrefix(n, lowerline) {
				c = append(c, n)
			}
		}
		if len(c) == 0 {
			if strings.HasPrefix(lowerline, "help ") {
				helpphrase := strings.TrimPrefix(lowerline, "help ")
				for _, n := range commandstrings {
					if strings.HasPrefix(n, helpphrase) {
						c = append(c, "help "+n)
					}
				}
			}
		}
		return
	})

	for {
		forceraw := false

		line, err := liner.Prompt(getPrompt())
		if err != nil {
			break
		}

		if len(line) == 0 {
			continue
		}

		parts, err := shellwords.Parse(line)

		if len(parts) == 0 {
			continue
		}

		liner.AppendHistory(line)

		if parts[0] == "help" {
			if len(parts) == 1 {
				fmt.Println("Enter help <command> to show information about a command")
				continue
			}
			lookup := parts[1]
			if len(parts) == 3 {
				lookup = parts[1] + " " + parts[2]
			}
			commanddata, ok := rediscommands[lookup]
			if ok {
				fmt.Printf("Command: %s\n", strings.ToUpper(lookup))
				fmt.Printf("Summary: %s\n", commanddata.Summary)
				if commanddata.Complexity != "" {
					fmt.Printf("Complexity: %s\n", commanddata.Complexity)
				}
				if commanddata.Arguments != nil {
					fmt.Println("Args:")
					for _, a := range commanddata.Arguments {
						fmt.Printf("     %s (%s)\n", a.Name, a.Type)
					}
				}
				continue
			}

		}

		if parts[0] == "exit" {
			break
		}

		if strings.ToLower(parts[0]) == "info" {
			forceraw = true
		}

		args := make([]interface{}, len(parts[1:]))
		for i, d := range parts[1:] {
			args[i] = d
		}

		catchMonitorCmd(conn, parts[0])

		result, err := conn.Do(parts[0], args...)

		printRedisResult(result, forceraw)
	}
}

// catchMonitorCmd to go into a "stream" mode to stream back
// every command processed by Redis server.
func catchMonitorCmd(conn redis.Conn, command string) {
	if strings.ToLower(command) == "monitor" {
		conn.Do("monitor")
		for {
			line, _ := redis.String(conn.Receive())
			fmt.Printf("%s\n", line)
		}
	}
}

func printRedisResult(result interface{}, forceraw bool) {
	printRedisResultIndenting(result, "", forceraw)
}

func printRedisResultIndenting(result interface{}, prefix string, forceraw bool) {
	switch v := result.(type) {
	case []interface{}:
		if raw || forceraw {
			for _, j := range v {
				switch vt := j.(type) {
				case []interface{}:
					printRedisResultIndenting(vt, "", forceraw)
				default:
					fmt.Printf("%s\n", toRedisValueString(vt, forceraw))
				}
			}
		} else {
			spacer := strings.Repeat(" ", len(prefix))
			for i, j := range v {
				switch vt := j.(type) {
				case []interface{}:
					newprefix := fmt.Sprintf("%s %d)", prefix, i+1)
					printRedisResultIndenting(vt, newprefix, forceraw)
				default:
					if i == 0 {
						fmt.Printf("%s %d) %s\n", prefix, i+1, toRedisValueString(j, forceraw))
					} else {
						fmt.Printf("%s %d) %s\n", spacer, i+1, toRedisValueString(j, forceraw))
					}
				}
			}
		}
	default:
		fmt.Printf("%s\n", toRedisValueString(result, forceraw))
	}
}

func toRedisValueString(value interface{}, forceraw bool) string {
	switch v := value.(type) {
	case redis.Error:
		if raw || forceraw {
			return fmt.Sprintf("%s", v.Error())
		}
		return fmt.Sprintf("(error) %s", v.Error())
	case int64:
		if raw || forceraw {
			return fmt.Sprintf("%d", v)
		}
		return fmt.Sprintf("(integer) %d", v)
	case string:
		return fmt.Sprintf("%s", v)
	case []byte:
		if raw || forceraw {
			return fmt.Sprintf("%s", string(v))
		}
		return fmt.Sprintf("\"%s\"", string(v))
	case nil:
		return "nil"
	}
	return ""
}

func getPrompt() string {
	if redisurlStr != "" {
		u, err := url.Parse(redisurlStr)
		if err == nil {
			return fmt.Sprintf("%s:%s> ", u.Hostname(), u.Port())
		}
	}

	return fmt.Sprintf("%s:%d> ", redishost, redisport)
}

// Commands is a holder for Redis Command structures
type Commands map[string]Command

// Command is a holder for Redis Command data includint arguments
type Command struct {
	Summary    string     `json:"summary"`
	Complexity string     `json:"complexity"`
	Arguments  []Argument `json:"arguments"`
	Since      string     `json:"since"`
	Group      string     `json:"group"`
}

// Argument is a holder for Redis Command Argument data
type Argument struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Enum     string `json:"enum,omitempty"`
	Optional bool   `json:"optional"`
}
