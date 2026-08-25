package main

import (
	"errors"
	"flag"
	"fmt"
	"net"
	"strconv"
	"strings"
)

const defaultAddress = "127.0.0.1:19091"

type config struct {
	Addr      string
	DataDir   string
	Selfcheck bool
}

func parseConfig(args []string, getenv func(string) string) (config, error) {
	fs := flag.NewFlagSet("stage-rig-clearance", flag.ContinueOnError)
	addr := fs.String("addr", "", "监听地址，格式为 127.0.0.1:<port>")
	data := fs.String("data", "data", "持久化数据目录")
	selfcheck := fs.Bool("selfcheck", false, "运行真实 HTTP 自检后退出")
	if err := fs.Parse(args); err != nil {
		return config{}, err
	}
	if fs.NArg() != 0 {
		return config{}, fmt.Errorf("unexpected arguments: %s", strings.Join(fs.Args(), " "))
	}
	resolved := *addr
	if resolved == "" {
		resolved = defaultAddress
		if raw := strings.TrimSpace(getenv("PORT")); raw != "" {
			port, err := strconv.Atoi(raw)
			if err != nil || port < 1 || port > 65535 {
				return config{}, errors.New("PORT 必须是 1-65535 的端口号")
			}
			resolved = net.JoinHostPort("127.0.0.1", strconv.Itoa(port))
		}
	}
	if err := validateAddress(resolved); err != nil {
		return config{}, err
	}
	if strings.TrimSpace(*data) == "" {
		return config{}, errors.New("data 目录不能为空")
	}
	return config{Addr: resolved, DataDir: *data, Selfcheck: *selfcheck}, nil
}

func validateAddress(address string) error {
	host, portText, err := net.SplitHostPort(address)
	if err != nil {
		return fmt.Errorf("addr 必须为 127.0.0.1:<port>: %w", err)
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return errors.New("addr 必须使用回环地址，禁止对外网卡监听")
	}
	port, err := strconv.Atoi(portText)
	if err != nil || port < 1 || port > 65535 {
		return errors.New("addr 端口必须在 1-65535 范围内")
	}
	return nil
}
