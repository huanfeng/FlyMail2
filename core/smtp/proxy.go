package smtp

import (
	"fmt"
	"net"
	"time"

	"flymail-core/types"

	"golang.org/x/net/proxy"
)

// dialProxy connects through a SOCKS5 or HTTP proxy.
func dialProxy(cfg *types.ProxyConfig, network, addr string) (net.Conn, error) {
	if cfg == nil || !cfg.Enabled() {
		return net.DialTimeout(network, addr, 10*time.Second)
	}

	switch cfg.Type {
	case "socks5":
		return dialSocks5(cfg, network, addr)
	default:
		return nil, fmt.Errorf("unsupported proxy type: %s", cfg.Type)
	}
}

func dialSocks5(cfg *types.ProxyConfig, network, addr string) (net.Conn, error) {
	proxyAddr := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)

	var auth *proxy.Auth
	if cfg.Username != "" {
		auth = &proxy.Auth{
			User:     cfg.Username,
			Password: cfg.Password,
		}
	}

	dialer, err := proxy.SOCKS5("tcp", proxyAddr, auth, proxy.Direct)
	if err != nil {
		return nil, fmt.Errorf("socks5 proxy init failed: %w", err)
	}
	return dialer.Dial(network, addr)
}
