package core

import (
	"fmt"
	"mail2im/internal/models"
	"net"
	"time"

	"golang.org/x/net/proxy"
)

type ProxyDialer struct {
	Proxy *models.Proxy
}

func NewProxyDialer(p *models.Proxy) *ProxyDialer {
	return &ProxyDialer{Proxy: p}
}

func (d *ProxyDialer) Dial(network, addr string) (net.Conn, error) {
	if d.Proxy == nil {
		return net.DialTimeout(network, addr, 10*time.Second)
	}

	if d.Proxy.Type == "socks5" {
		auth := &proxy.Auth{
			User:     d.Proxy.Username,
			Password: d.Proxy.Password, // Should be decrypted
		}
		if d.Proxy.Username == "" {
			auth = nil
		}

		proxyAddr := fmt.Sprintf("%s:%d", d.Proxy.Host, d.Proxy.Port)
		dialer, err := proxy.SOCKS5("tcp", proxyAddr, auth, proxy.Direct)
		if err != nil {
			return nil, err
		}
		return dialer.Dial(network, addr)
	}

	// TODO: Implement HTTP proxy support if needed
	return nil, fmt.Errorf("unsupported proxy type: %s", d.Proxy.Type)
}
