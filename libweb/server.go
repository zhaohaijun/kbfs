package libweb

import (
	"crypto/tls"
	"net"
)

type ServerConfig struct {
	AutoDirectHTTPAddrs []string
	HTTPSAddr           string
}

type Server struct {
	config ServerConfig

	httpListener  net.Listener
	httpsListener net.Listener
}

func listenHTTPS(addr string) (net.Listener, error) {
	return tls.Listen("tcp", addr, &tls.Config{
		NextProtos: []string{"h2", "http/1.1"},
		GetCertificate: func(hello *tls.ClientHelloInfo) (
			cert *tls.Certificate, err error) {
			if _, err = loadConfigFromDNS(hello.ServerName); err != nil {
				return nil, err
			}
		},
	})
}

func ListenAndServe(config ServerConfig) (server *Server, err error) {
	server = &Server{config: config}

	if server.config.autoDirectHTTP {
		if server.httpListener, err = net.Listen("tcp", ":80"); err != nil {
			return nil, err
		}
	}
	return server, nil
}
