package connectorlib

import (
	"log"
	"net"
	"sni/util/env"
	"time"
)

func StartClient() (err error) {
	connectHost := env.GetOrDefault("SNI_CONNECTORLIB_HOST", "127.0.0.1")
	connectPort := env.GetOrDefault("SNI_CONNECTORLIB_PORT", "43884")
	connectHostPort := net.JoinHostPort(connectHost, connectPort)

	var connectAddr *net.TCPAddr
	connectAddr, err = net.ResolveTCPAddr("tcp", connectHostPort)
	if err != nil {
		return
	}

	go connectLoop(connectAddr, connectHostPort)

	return
}

func connectLoop(connectAddr *net.TCPAddr, connectHostPort string) {
	var err error
	var conn *net.TCPConn

	for {
		count := 0
		for {
			conn, err = net.DialTCP("tcp", nil, connectAddr)
			if err == nil {
				break
			}

			if count == 0 {
				log.Printf("connectorlib: could not connect to %s; error: %v\n", connectHostPort, err)
			}
			count++
			if count >= 30 {
				count = 0
			}

			time.Sleep(time.Second)
		}

		client := &Client{c: conn}
		client.handleRead()
	}
}
