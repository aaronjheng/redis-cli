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
	"net/url"
	"os"
	"strconv"

	redigo "github.com/gomodule/redigo/redis"
	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"

	"github.com/aaronjheng/redis-cli/internal/config"
	internalredis "github.com/aaronjheng/redis-cli/internal/redis"
	"github.com/aaronjheng/redis-cli/internal/ssh"
)

const defaultRedisPort = 6379

//nolint:gochecknoglobals // Cobra command wiring keeps shared CLI state here.
var (
	cfg     *config.Config
	profile string
)

type commandConfig struct {
	dialConfig internalredis.DialConfig
	redisURI   string
	redisHost  string
	redisPort  int
	rawOutput  bool
	evalFile   string
}

func rootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:               "redis",
		Short:             "A Redis CLI client",
		SilenceUsage:      true,
		Args:              cobra.ArbitraryArgs,
		PersistentPreRunE: persistentPreRunE,
		RunE:              runE,
	}

	cmd.SetHelpCommand(&cobra.Command{Hidden: true})
	cmd.AddCommand(completionCmd())
	cmd.Flags().SortFlags = false

	cmd.PersistentFlags().StringVarP(&profile, "profile", "P", "", "Profile name to connect to.")
	cmd.PersistentFlags().StringP("config", "f", "", "Config file path.")

	err := cmd.RegisterFlagCompletionFunc("profile", profileCompletionFunc)
	if err != nil {
		panic(fmt.Sprintf("register profile completion: %v", err))
	}

	registerRootFlags(cmd)

	return cmd
}

func persistentPreRunE(cmd *cobra.Command, _ []string) error {
	if _, ok := cmd.Annotations["skipConfigLoad"]; ok {
		return nil
	}

	cfgFilepath, err := cmd.Flags().GetString("config")
	if err != nil {
		return fmt.Errorf("config flag: %w", err)
	}

	cfg, err = config.LoadConfig(cfgFilepath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	return nil
}

func registerRootFlags(cmd *cobra.Command) {
	cmd.Flags().StringP("uri", "u", "", "URI to connect to")
	cmd.Flags().StringP("host", "h", "127.0.0.1", "Host to connect to")
	cmd.Flags().IntP("port", "p", defaultRedisPort, "Port to connect to")
	cmd.Flags().IntP("db", "n", 0, "Redis database to access")
	cmd.Flags().BoolP("cluster", "c", false, "Force cluster mode")
	cmd.Flags().StringP("user", "r", "", "Username to use when connecting. Supported since Redis 6.")
	cmd.Flags().StringP("password", "a", "", "Password to use when connecting")
	cmd.Flags().Bool("tls", false, "Enable TLS/SSL")
	cmd.Flags().StringP("sni", "s", "", "Server Name Indication for TLS certificate verification")
	cmd.Flags().Bool("insecure", false, "Disable certificate validation")
	cmd.Flags().String("cacert", "", "CA certificate file for validation")
	cmd.Flags().String("certb64", "", "Self-signed certificate string as base64 for validation")
	cmd.Flags().String("ssh", "", "SSH tunnel connection URI. Format: [user[:pass]@]host[:port]")
	cmd.Flags().String("ssh-identity-file", "", "SSH identity file")
	cmd.Flags().Bool("raw", false, "Produce raw output")
	cmd.Flags().String("eval", "", "Evaluate a Lua script file, follow with keys a , and args")

	cmd.Version = version
	cmd.InitDefaultVersionFlag()
	cmd.Flags().Lookup("version").Usage = "Print version"

	cmd.Flags().Bool("help", false, "Help for redis")
}

func main() {
	err := rootCmd().Execute()
	if err != nil {
		os.Exit(1)
	}
}

func runE(cmd *cobra.Command, args []string) error {
	cmdCfg, err := commandConfigFromFlags(cmd)
	if err != nil {
		return err
	}

	conn, err := internalredis.Dial(cmdCfg.dialConfig)
	if err != nil {
		return fmt.Errorf("dial: %w", err)
	}

	defer conn.Close()

	printer := &internalredis.Printer{Raw: cmdCfg.rawOutput}

	return runRedisAction(conn, cmdCfg, args, printer)
}

func commandConfigFromFlags(cmd *cobra.Command) (commandConfig, error) {
	profCfg, err := resolveProfileConfig()
	if err != nil {
		return commandConfig{}, err
	}

	uri := overrideString(cmd, "uri", profCfg.URI)
	host := overrideString(cmd, "host", profCfg.Host)
	port := overrideInt(cmd, "port", profCfg.Port)
	user := overrideString(cmd, "user", profCfg.User)
	password := overrideString(cmd, "password", profCfg.Password)
	redisDB := overrideInt(cmd, "db", profCfg.DB)
	cluster := overrideBool(cmd, "cluster", profCfg.Cluster)
	enableTLS := overrideBool(cmd, "tls", profCfg.TLS)
	serverName := overrideString(cmd, "sni", profCfg.ServerName)
	insecure := overrideBool(cmd, "insecure", profCfg.Insecure)

	sshCfg, err := resolveSSHConfig(cmd, profCfg)
	if err != nil {
		return commandConfig{}, err
	}

	caCertFile, certB64 := loadCert(cmd, profCfg)
	forceRaw, _ := cmd.Flags().GetBool("raw")
	evalFile, _ := cmd.Flags().GetString("eval")

	raw := forceRaw || !isatty.IsTerminal(os.Stdout.Fd()) && !isatty.IsCygwinTerminal(os.Stdout.Fd())

	cert, err := internalredis.LoadCert(caCertFile, certB64)
	if err != nil {
		return commandConfig{}, fmt.Errorf("load cert: %w", err)
	}

	return commandConfig{
		dialConfig: internalredis.DialConfig{
			URI:        uri,
			Host:       host,
			Port:       port,
			User:       user,
			Password:   password,
			DB:         redisDB,
			Cluster:    cluster,
			TLS:        enableTLS,
			ServerName: serverName,
			Insecure:   insecure,
			SSH:        sshCfg,
			Cert:       cert,
		},
		redisURI:  uri,
		redisHost: host,
		redisPort: port,
		rawOutput: raw,
		evalFile:  evalFile,
	}, nil
}

func resolveProfileConfig() (*config.ProfileConfig, error) {
	if cfg == nil {
		return &config.ProfileConfig{}, nil //nolint:exhaustruct // zero values trigger flag defaults
	}

	profCfg, err := cfg.Profile(profile)
	if err != nil {
		return nil, fmt.Errorf("resolve profile: %w", err)
	}

	return profCfg, nil
}

func resolveSSHConfig(cmd *cobra.Command, profCfg *config.ProfileConfig) (*ssh.Config, error) {
	if cmd.Flags().Changed("ssh") {
		sshURI, _ := cmd.Flags().GetString("ssh")
		sshIdentityFile, _ := cmd.Flags().GetString("ssh-identity-file")

		return parseSSHURI(sshURI, sshIdentityFile)
	}

	if profCfg.SSH == nil {
		return nil, nil //nolint:nilnil // nil means no SSH configured
	}

	sshCfg := *profCfg.SSH

	if cmd.Flags().Changed("ssh-identity-file") {
		sshCfg.IdentityFile, _ = cmd.Flags().GetString("ssh-identity-file")
	}

	return &sshCfg, nil
}

func parseSSHURI(sshURI, sshIdentityFile string) (*ssh.Config, error) {
	sshURL, err := url.Parse("ssh://" + sshURI)
	if err != nil {
		return nil, fmt.Errorf("parse SSH URI: %w", err)
	}

	port := sshURL.Port()
	if port == "" {
		port = "22"
	}

	portNum, err := strconv.ParseInt(port, 10, 32)
	if err != nil {
		return nil, fmt.Errorf("parse SSH port: %w", err)
	}

	return &ssh.Config{
		Host:         sshURL.Hostname(),
		Port:         int32(portNum),
		User:         sshURL.User.Username(),
		IdentityFile: sshIdentityFile,
	}, nil
}

func overrideString(cmd *cobra.Command, name, profileVal string) string {
	if cmd.Flags().Changed(name) {
		v, _ := cmd.Flags().GetString(name)

		return v
	}

	if profileVal != "" {
		return profileVal
	}

	v, _ := cmd.Flags().GetString(name)

	return v
}

func overrideInt(cmd *cobra.Command, name string, profileVal int) int {
	if cmd.Flags().Changed(name) {
		v, _ := cmd.Flags().GetInt(name)

		return v
	}

	if profileVal != 0 {
		return profileVal
	}

	v, _ := cmd.Flags().GetInt(name)

	return v
}

func overrideBool(cmd *cobra.Command, name string, profileVal bool) bool {
	if cmd.Flags().Changed(name) {
		v, _ := cmd.Flags().GetBool(name)

		return v
	}

	if profileVal {
		return true
	}

	v, _ := cmd.Flags().GetBool(name)

	return v
}

func runRedisAction(
	conn redigo.Conn,
	cmdCfg commandConfig,
	args []string,
	printer *internalredis.Printer,
) error {
	if cmdCfg.evalFile != "" {
		err := internalredis.RunEvalScript(conn, cmdCfg.evalFile, args, printer)
		if err != nil {
			return fmt.Errorf("run eval script: %w", err)
		}

		return nil
	}

	if len(args) > 0 {
		err := internalredis.RunCommand(conn, args, printer)
		if err != nil {
			return fmt.Errorf("run command: %w", err)
		}

		return nil
	}

	err := internalredis.RunInteractive(conn, cmdCfg.redisURI, cmdCfg.redisHost, cmdCfg.redisPort, printer)
	if err != nil {
		return fmt.Errorf("run interactive: %w", err)
	}

	return nil
}

func loadCert(cmd *cobra.Command, profCfg *config.ProfileConfig) (string, string) {
	caCertFile, _ := cmd.Flags().GetString("cacert")
	certB64, _ := cmd.Flags().GetString("certb64")

	if !cmd.Flags().Changed("cacert") {
		if val := os.Getenv("REDIS_CACERT"); val != "" {
			caCertFile = val
		} else if profCfg != nil {
			caCertFile = profCfg.CACert
		}
	}

	if !cmd.Flags().Changed("certb64") {
		if val := os.Getenv("REDIS_CERTB64"); val != "" {
			certB64 = val
		} else if profCfg != nil {
			certB64 = profCfg.CertB64
		}
	}

	return caCertFile, certB64
}
