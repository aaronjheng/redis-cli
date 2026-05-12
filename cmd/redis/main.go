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
	"fmt"
	"os"

	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"

	rediscommands "github.com/aaronjheng/redis-cli/internal/redis"
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

var raw = false //nolint:gochecknoglobals

func rootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:          "redis",
		Short:        "A Redis CLI client",
		SilenceUsage: true,
		Args:         cobra.ArbitraryArgs,
		RunE:         runE,
	}

	cmd.SetHelpCommand(&cobra.Command{Hidden: true})
	cmd.AddCommand(completionCmd())
	cmd.Flags().SortFlags = false

	cmd.Flags().StringVarP(&redisurlStr, "uri", "u", "",
		"URI to connect to")
	cmd.Flags().StringVarP(&redishost, "host", "h", "127.0.0.1",
		"Host to connect to")
	cmd.Flags().IntVarP(&redisport, "port", "p", defaultRedisPort,
		"Port to connect to")
	cmd.Flags().IntVarP(&redisdb, "db", "n", 0,
		"Redis database to access")
	cmd.Flags().StringVarP(&user, "user", "r", "",
		"Username to use when connecting. Supported since Redis 6.")
	cmd.Flags().StringVarP(&redisauth, "password", "a", "",
		"Password to use when connecting")
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
	cmd.Flags().StringVar(&sshURI, "ssh", "",
		"SSH tunnel connection URI. Format: [user[:pass]@]host[:port]")
	cmd.Flags().StringVar(&sshIdentityFile, "ssh-identity-file", "",
		"SSH identity file")
	cmd.Flags().BoolVar(&forceraw, "raw", false,
		"Produce raw output")
	cmd.Flags().StringVar(&evalFile, "eval", "",
		"Evaluate a Lua script file, follow with keys a , and args")

	cmd.Version = version
	cmd.InitDefaultVersionFlag()
	cmd.Flags().Lookup("version").Usage = "Print version"

	cmd.Flags().Bool("help", false, "Help for redis")

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

	cert, err := rediscommands.LoadCert(cacertfile, rediscertb64)
	if err != nil {
		return fmt.Errorf("load cert: %w", err)
	}

	conn, err := rediscommands.Dial(rediscommands.DialConfig{
		URI:             redisurlStr,
		Host:            redishost,
		Port:            redisport,
		User:            user,
		Password:        redisauth,
		DB:              redisdb,
		TLS:             redistls,
		ServerName:      servername,
		Insecure:        insecure,
		SSHURI:          sshURI,
		SSHIdentityFile: sshIdentityFile,
		Cert:            cert,
	})
	if err != nil {
		return fmt.Errorf("dial: %w", err)
	}

	defer conn.Close()

	printer := &rediscommands.Printer{Raw: raw}

	if evalFile != "" {
		return fmt.Errorf("run eval script: %w", rediscommands.RunEvalScript(conn, evalFile, commandargs, printer))
	}

	if len(commandargs) > 0 {
		return fmt.Errorf("run command: %w", rediscommands.RunCommand(conn, commandargs, printer))
	}

	return fmt.Errorf(
		"run interactive: %w",
		rediscommands.RunInteractive(conn, redisurlStr, redishost, redisport, printer),
	)
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
