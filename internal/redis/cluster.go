package redis

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"

	"github.com/gomodule/redigo/redis"
)

const (
	clusterSlotCount             = 16384
	clusterRedirectRetries       = 8
	clusterRedirectFieldCount    = 3
	crc16BitsPerByte             = 8
	crc16Polynomial              = 0x1021
	minKeyedScriptCommandArgs    = 3
	xreadGroupStreamsSearchIndex = 3
)

var (
	errClusterDBNotZero         = errors.New("redis cluster supports database 0 only")
	errTooManyClusterRedirects  = errors.New("too many cluster redirects")
	errClusterSlotsArray        = errors.New("parse cluster slots: expected array reply")
	errClusterSlotsRange        = errors.New("parse cluster slots: expected slot range")
	errClusterSlotsInvalidRange = errors.New("parse cluster slots: invalid range")
	errClusterSlotsMasterNode   = errors.New("parse cluster slots: expected master node")
	errEmptyRedisAddress        = errors.New("empty redis address")
	errRedisAddressMissingPort  = errors.New("redis address is missing a port")
)

type clusterConn struct {
	seedURL     *url.URL
	seedAddr    string
	dialOptions []redis.DialOption
	conns       map[string]redis.Conn
	slots       [clusterSlotCount]string
}

type redisCommandError struct {
	err error
}

func (e redisCommandError) Error() string {
	return e.err.Error()
}

func (e redisCommandError) Unwrap() error {
	return e.err
}

func dialCluster(cfg DialConfig) (*clusterConn, error) {
	err := validateClusterDB(cfg)
	if err != nil {
		return nil, err
	}

	connectionURL := buildConnectionURL(cfg)

	parsedURL, err := url.Parse(connectionURL)
	if err != nil {
		return nil, fmt.Errorf("parse redis URL: %w", err)
	}

	parsedURL.Path = "/0"

	seedAddr, err := normalizeAddress(parsedURL.Host, parsedURL.Hostname())
	if err != nil {
		return nil, fmt.Errorf("parse seed address: %w", err)
	}

	parsedURL.Host = seedAddr

	dialOptions, err := buildDialOptions(cfg)
	if err != nil {
		return nil, err
	}

	conn, err := dialRedis(parsedURL.String(), dialOptions)
	if err != nil {
		return nil, fmt.Errorf("dial seed node: %w", err)
	}

	clusterConn := &clusterConn{
		seedURL:     parsedURL,
		seedAddr:    seedAddr,
		dialOptions: dialOptions,
		conns: map[string]redis.Conn{
			seedAddr: conn,
		},
		slots: [clusterSlotCount]string{},
	}

	err = clusterConn.refreshSlots(conn)
	if err != nil {
		_ = clusterConn.Close()

		return nil, err
	}

	return clusterConn, nil
}

func validateClusterDB(cfg DialConfig) error {
	if cfg.URI == "" {
		if cfg.DB != 0 {
			return fmt.Errorf("%w: got database %d", errClusterDBNotZero, cfg.DB)
		}

		return nil
	}

	parsedURL, err := url.Parse(cfg.URI)
	if err != nil {
		return fmt.Errorf("parse redis URL: %w", err)
	}

	database, err := dbFromURLPath(parsedURL.Path)
	if err != nil {
		return err
	}

	if database != 0 {
		return fmt.Errorf("%w: got database %d", errClusterDBNotZero, database)
	}

	return nil
}

func dbFromURLPath(path string) (int, error) {
	path = strings.TrimPrefix(path, "/")
	if path == "" {
		return 0, nil
	}

	db, err := strconv.Atoi(path)
	if err != nil {
		return 0, fmt.Errorf("parse redis database from URI: %w", err)
	}

	return db, nil
}

func (c *clusterConn) Close() error {
	var closeErr error

	for _, conn := range c.conns {
		err := conn.Close()
		if err != nil && closeErr == nil {
			closeErr = err
		}
	}

	return closeErr
}

func (c *clusterConn) Err() error {
	for _, conn := range c.conns {
		err := conn.Err()
		if err != nil {
			return fmt.Errorf("cluster connection error: %w", err)
		}
	}

	return nil
}

func (c *clusterConn) Do(commandName string, args ...any) (any, error) {
	addr := c.routeAddress(commandName, args)
	asking := false

	for range clusterRedirectRetries {
		conn, err := c.connForAddress(addr)
		if err != nil {
			return nil, err
		}

		if asking {
			_, err = conn.Do("ASKING")
			if err != nil {
				return nil, fmt.Errorf("send ASKING: %w", err)
			}
		}

		reply, err := conn.Do(commandName, args...)
		if err == nil {
			return reply, nil
		}

		redirect, ok := parseClusterRedirect(err, addr)
		if !ok {
			return nil, redisCommandError{err: err}
		}

		addr = redirect.addr

		asking = redirect.kind == "ASK"
		if redirect.kind == "MOVED" && redirect.slot >= 0 && redirect.slot < clusterSlotCount {
			c.slots[redirect.slot] = redirect.addr
		}
	}

	return nil, errTooManyClusterRedirects
}

func (c *clusterConn) Send(commandName string, args ...any) error {
	conn, err := c.connForAddress(c.seedAddr)
	if err != nil {
		return err
	}

	err = conn.Send(commandName, args...)
	if err != nil {
		return fmt.Errorf("send command: %w", err)
	}

	return nil
}

func (c *clusterConn) Flush() error {
	conn, err := c.connForAddress(c.seedAddr)
	if err != nil {
		return err
	}

	err = conn.Flush()
	if err != nil {
		return fmt.Errorf("flush command buffer: %w", err)
	}

	return nil
}

func (c *clusterConn) Receive() (any, error) {
	conn, err := c.connForAddress(c.seedAddr)
	if err != nil {
		return nil, err
	}

	reply, err := conn.Receive()
	if err != nil {
		return nil, fmt.Errorf("receive reply: %w", err)
	}

	return reply, nil
}

func (c *clusterConn) routeAddress(commandName string, args []any) string {
	key, ok := firstCommandKey(commandName, args)
	if !ok {
		return c.seedAddr
	}

	slot := keySlot(key)
	if addr := c.slots[slot]; addr != "" {
		return addr
	}

	return c.seedAddr
}

//nolint:ireturn
func (c *clusterConn) connForAddress(addr string) (redis.Conn, error) {
	addr, err := normalizeAddress(addr, c.seedURL.Hostname())
	if err != nil {
		return nil, err
	}

	if conn, ok := c.conns[addr]; ok {
		return conn, nil
	}

	nodeURL := *c.seedURL
	nodeURL.Host = addr
	nodeURL.Path = "/0"

	conn, err := dialRedis(nodeURL.String(), c.dialOptions)
	if err != nil {
		return nil, fmt.Errorf("dial cluster node %s: %w", addr, err)
	}

	c.conns[addr] = conn

	return conn, nil
}

func (c *clusterConn) refreshSlots(conn redis.Conn) error {
	reply, err := conn.Do("CLUSTER", "SLOTS")
	if err != nil {
		return fmt.Errorf("load cluster slots: %w", err)
	}

	slots, err := parseClusterSlots(reply, c.seedURL.Hostname())
	if err != nil {
		return err
	}

	c.slots = slots

	return nil
}

func parseClusterSlots(reply any, fallbackHost string) ([clusterSlotCount]string, error) {
	var slots [clusterSlotCount]string

	ranges, ok := reply.([]any)
	if !ok {
		return slots, errClusterSlotsArray
	}

	for _, item := range ranges {
		start, end, addr, err := parseClusterSlotRange(item, fallbackHost)
		if err != nil {
			return slots, err
		}

		for slot := start; slot <= end; slot++ {
			slots[slot] = addr
		}
	}

	return slots, nil
}

func parseClusterSlotRange(item any, fallbackHost string) (int, int, string, error) {
	slotRange, ok := item.([]any)
	if !ok || len(slotRange) < 3 {
		return 0, 0, "", errClusterSlotsRange
	}

	start, end, err := parseClusterSlotBounds(slotRange)
	if err != nil {
		return 0, 0, "", err
	}

	addr, err := parseClusterMasterAddress(slotRange[2], fallbackHost)
	if err != nil {
		return 0, 0, "", err
	}

	return start, end, addr, nil
}

func parseClusterSlotBounds(slotRange []any) (int, int, error) {
	start, err := redis.Int(slotRange[0], nil)
	if err != nil {
		return 0, 0, fmt.Errorf("parse cluster slot start: %w", err)
	}

	end, err := redis.Int(slotRange[1], nil)
	if err != nil {
		return 0, 0, fmt.Errorf("parse cluster slot end: %w", err)
	}

	if start < 0 || end >= clusterSlotCount || start > end {
		return 0, 0, fmt.Errorf("%w: %d-%d", errClusterSlotsInvalidRange, start, end)
	}

	return start, end, nil
}

func parseClusterMasterAddress(nodeReply any, fallbackHost string) (string, error) {
	node, ok := nodeReply.([]any)
	if !ok || len(node) < 2 {
		return "", errClusterSlotsMasterNode
	}

	host, err := clusterNodeHost(node[0], fallbackHost)
	if err != nil {
		return "", fmt.Errorf("parse cluster node host: %w", err)
	}

	port, err := redis.Int(node[1], nil)
	if err != nil {
		return "", fmt.Errorf("parse cluster node port: %w", err)
	}

	addr, err := normalizeAddress(net.JoinHostPort(host, strconv.Itoa(port)), fallbackHost)
	if err != nil {
		return "", err
	}

	return addr, nil
}

func clusterNodeHost(value any, fallbackHost string) (string, error) {
	host, err := redis.String(value, nil)
	if err != nil && !errors.Is(err, redis.ErrNil) {
		return "", fmt.Errorf("parse redis string: %w", err)
	}

	if host == "" || host == "?" {
		return fallbackHost, nil
	}

	return host, nil
}

type clusterRedirect struct {
	kind string
	slot int
	addr string
}

func parseClusterRedirect(err error, fallbackAddr string) (clusterRedirect, bool) {
	if err == nil {
		return clusterRedirect{kind: "", slot: 0, addr: ""}, false
	}

	message := err.Error()

	fields := strings.Fields(message)
	if len(fields) != clusterRedirectFieldCount {
		return clusterRedirect{kind: "", slot: 0, addr: ""}, false
	}

	kind := fields[0]
	if kind != "MOVED" && kind != "ASK" {
		return clusterRedirect{kind: "", slot: 0, addr: ""}, false
	}

	slot, err := strconv.Atoi(fields[1])
	if err != nil {
		return clusterRedirect{kind: "", slot: 0, addr: ""}, false
	}

	fallbackHost, _, splitErr := net.SplitHostPort(fallbackAddr)
	if splitErr != nil {
		fallbackHost = fallbackAddr
	}

	addr, err := normalizeAddress(fields[2], fallbackHost)
	if err != nil {
		return clusterRedirect{kind: "", slot: 0, addr: ""}, false
	}

	return clusterRedirect{kind: kind, slot: slot, addr: addr}, true
}

func normalizeAddress(rawAddr, fallbackHost string) (string, error) {
	if rawAddr == "" {
		return "", errEmptyRedisAddress
	}

	if strings.HasPrefix(rawAddr, ":") {
		rawAddr = fallbackHost + rawAddr
	}

	host, port, err := net.SplitHostPort(rawAddr)
	if err == nil {
		if host == "" {
			host = fallbackHost
		}

		return net.JoinHostPort(host, port), nil
	}

	host, port, found := strings.Cut(rawAddr, ":")
	if !found {
		return "", fmt.Errorf("%w: %q", errRedisAddressMissingPort, rawAddr)
	}

	if host == "" {
		host = fallbackHost
	}

	return net.JoinHostPort(host, port), nil
}

func firstCommandKey(commandName string, args []any) (string, bool) {
	if len(args) == 0 {
		return "", false
	}

	command := strings.ToLower(commandName)
	switch command {
	case "eval", "evalsha":
		return scriptCommandKey(args)
	case "fcall", "fcall_ro":
		return functionCommandKey(args)
	case "xread":
		return streamsCommandKey(args, 0)
	case "xreadgroup":
		return streamsCommandKey(args, xreadGroupStreamsSearchIndex)
	}

	return argString(args[0])
}

func scriptCommandKey(args []any) (string, bool) {
	if len(args) < minKeyedScriptCommandArgs {
		return "", false
	}

	keyCount, ok := argInt(args[1])
	if !ok || keyCount == 0 {
		return "", false
	}

	return argString(args[2])
}

func functionCommandKey(args []any) (string, bool) {
	if len(args) < minKeyedScriptCommandArgs {
		return "", false
	}

	keyCount, ok := argInt(args[1])
	if !ok || keyCount == 0 {
		return "", false
	}

	return argString(args[2])
}

func streamsCommandKey(args []any, minStreamsIndex int) (string, bool) {
	for idx, arg := range args {
		if idx < minStreamsIndex {
			continue
		}

		text, ok := argString(arg)
		if !ok {
			continue
		}

		if strings.EqualFold(text, "STREAMS") && idx+1 < len(args) {
			return argString(args[idx+1])
		}
	}

	return "", false
}

func argString(value any) (string, bool) {
	switch val := value.(type) {
	case string:
		return val, true
	case []byte:
		return string(val), true
	default:
		return fmt.Sprint(val), true
	}
}

func argInt(value any) (int, bool) {
	switch val := value.(type) {
	case int:
		return val, true
	case int64:
		return int(val), true
	case string:
		parsed, err := strconv.Atoi(val)

		return parsed, err == nil
	case []byte:
		parsed, err := strconv.Atoi(string(val))

		return parsed, err == nil
	default:
		parsed, err := strconv.Atoi(fmt.Sprint(val))

		return parsed, err == nil
	}
}

func keySlot(key string) int {
	return crc16([]byte(hashTag(key))) % clusterSlotCount
}

func hashTag(key string) string {
	start := strings.IndexByte(key, '{')
	if start == -1 {
		return key
	}

	end := strings.IndexByte(key[start+1:], '}')
	if end <= 0 {
		return key
	}

	return key[start+1 : start+1+end]
}

func crc16(data []byte) int {
	var crc uint16

	for _, value := range data {
		crc ^= uint16(value) << crc16BitsPerByte
		for range crc16BitsPerByte {
			if crc&0x8000 != 0 {
				crc = (crc << 1) ^ crc16Polynomial
			} else {
				crc <<= 1
			}
		}
	}

	return int(crc)
}
