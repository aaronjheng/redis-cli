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

	cmd.Flags().StringP("uri", "u", "",
		"URI to connect to")
	cmd.Flags().StringP("host", "h", "127.0.0.1",
		"Host to connect to")
	cmd.Flags().IntP("port", "p", defaultRedisPort,
		"Port to connect to")
	cmd.Flags().IntP("db", "n", 0,
		"Redis database to access")
	cmd.Flags().StringP("user", "r", "",
		"Username to use when connecting. Supported since Redis 6.")
	cmd.Flags().StringP("password", "a", "",
		"Password to use when connecting")
	cmd.Flags().Bool("tls", false,
		"Enable TLS/SSL")
	cmd.Flags().StringP("sni", "s", "",
		"Server Name Indication for TLS certificate verification")
	cmd.Flags().Bool("insecure", false,
		"Disable certificate validation")
	cmd.Flags().String("cacert", "",
		"CA certificate file for validation")
	cmd.Flags().String("certb64", "",
		"Self-signed certificate string as base64 for validation")
	cmd.Flags().String("ssh", "",
		"SSH tunnel connection URI. Format: [user[:pass]@]host[:port]")
	cmd.Flags().String("ssh-identity-file", "",
		"SSH identity file")
	cmd.Flags().Bool("raw", false,
		"Produce raw output")
	cmd.Flags().String("eval", "",
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
	uri, _ := cmd.Flags().GetString("uri")
	host, _ := cmd.Flags().GetString("host")
	port, _ := cmd.Flags().GetInt("port")
	user, _ := cmd.Flags().GetString("user")
	password, _ := cmd.Flags().GetString("password")
	redisDB, _ := cmd.Flags().GetInt("db")
	enableTLS, _ := cmd.Flags().GetBool("tls")
	serverName, _ := cmd.Flags().GetString("sni")
	insecure, _ := cmd.Flags().GetBool("insecure")
	caCertFile, certB64 := loadCertFromEnv(cmd)
	sshURI, _ := cmd.Flags().GetString("ssh")
	sshIdentityFile, _ := cmd.Flags().GetString("ssh-identity-file")
	forceRaw, _ := cmd.Flags().GetBool("raw")
	evalFile, _ := cmd.Flags().GetString("eval")

	raw := forceRaw || !isatty.IsTerminal(os.Stdout.Fd()) && !isatty.IsCygwinTerminal(os.Stdout.Fd())

	cert, err := rediscommands.LoadCert(caCertFile, certB64)
	if err != nil {
		return fmt.Errorf("load cert: %w", err)
	}

	conn, err := rediscommands.Dial(rediscommands.DialConfig{
		URI:             uri,
		Host:            host,
		Port:            port,
		User:            user,
		Password:        password,
		DB:              redisDB,
		TLS:             enableTLS,
		ServerName:      serverName,
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
		return fmt.Errorf("run eval script: %w", rediscommands.RunEvalScript(conn, evalFile, args, printer))
	}

	if len(args) > 0 {
		return fmt.Errorf("run command: %w", rediscommands.RunCommand(conn, args, printer))
	}

	return fmt.Errorf(
		"run interactive: %w",
		rediscommands.RunInteractive(conn, uri, host, port, printer),
	)
}

func loadCertFromEnv(cmd *cobra.Command) (string, string) {
	caCertFile, _ := cmd.Flags().GetString("cacert")
	certB64, _ := cmd.Flags().GetString("certb64")

	if !cmd.Flags().Changed("cacert") {
		if val := os.Getenv("REDIS_CACERT"); val != "" {
			caCertFile = val
		}
	}

	if !cmd.Flags().Changed("certb64") {
		if val := os.Getenv("REDIS_CERTB64"); val != "" {
			certB64 = val
		}
	}

	return caCertFile, certB64
}
