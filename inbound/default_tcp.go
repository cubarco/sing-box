package inbound

import (
	"context"
	"net"
	"time"

	"github.com/sagernet/sing-box/adapter"
	"github.com/sagernet/sing-box/common/proxyproto"
	"github.com/sagernet/sing-box/log"
	E "github.com/sagernet/sing/common/exceptions"
	M "github.com/sagernet/sing/common/metadata"
	N "github.com/sagernet/sing/common/network"
	"github.com/sagernet/tfo-go"
)

type customListener struct {
	net.Listener
	log.Logger
}

func (cl *customListener) Accept() (net.Conn, error) {
	conn, err := cl.Listener.Accept()
	if err != nil {
		return nil, err
	}
	cl.Logger.Info("xxxxxxxxx setting keepalive for real")
	conn.(*net.TCPConn).SetKeepAlive(true)
	conn.(*net.TCPConn).SetKeepAlivePeriod(3600 * time.Second)
	return conn, nil
}

func (a *myInboundAdapter) ListenTCP() (net.Listener, error) {
	var err error
	bindAddr := M.SocksaddrFrom(a.listenOptions.Listen.Build(), a.listenOptions.ListenPort)
	var tcpListener net.Listener
	if !a.listenOptions.TCPFastOpen {
		tcpListener, err = net.ListenTCP(M.NetworkFromNetAddr(N.NetworkTCP, bindAddr.Addr), bindAddr.TCPAddr())
	} else {
		tcpListener, err = tfo.ListenTCP(M.NetworkFromNetAddr(N.NetworkTCP, bindAddr.Addr), bindAddr.TCPAddr())
	}
	a.logger.Info("xxxxxxxx setting listener")
	tcpListener = &customListener{Listener: tcpListener, Logger: a.logger}
	if err == nil {
		a.logger.Info("tcp server started at ", tcpListener.Addr())
	}
	if a.listenOptions.ProxyProtocol {
		a.logger.Debug("proxy protocol enabled")
		tcpListener = &proxyproto.Listener{Listener: tcpListener, AcceptNoHeader: a.listenOptions.ProxyProtocolAcceptNoHeader}
	}
	a.tcpListener = tcpListener
	return tcpListener, err
}

func (a *myInboundAdapter) loopTCPIn() {
	tcpListener := a.tcpListener
	for {
		conn, err := tcpListener.Accept()
		if err != nil {
			//goland:noinspection GoDeprecation
			//nolint:staticcheck
			if netError, isNetError := err.(net.Error); isNetError && netError.Temporary() {
				a.logger.Error(err)
				continue
			}
			if a.inShutdown.Load() && E.IsClosed(err) {
				return
			}
			a.tcpListener.Close()
			a.logger.Error("serve error: ", err)
			continue
		}
		go a.injectTCP(conn, adapter.InboundContext{})
	}
}

func (a *myInboundAdapter) injectTCP(conn net.Conn, metadata adapter.InboundContext) {
	ctx := log.ContextWithNewID(a.ctx)
	metadata = a.createMetadata(conn, metadata)
	a.logger.InfoContext(ctx, "inbound connection from ", metadata.Source)
	hErr := a.connHandler.NewConnection(ctx, conn, metadata)
	if hErr != nil {
		conn.Close()
		a.NewError(ctx, E.Cause(hErr, "process connection from ", metadata.Source))
	}
}

func (a *myInboundAdapter) routeTCP(ctx context.Context, conn net.Conn, metadata adapter.InboundContext) {
	a.logger.InfoContext(ctx, "inbound connection from ", metadata.Source)
	hErr := a.newConnection(ctx, conn, metadata)
	if hErr != nil {
		conn.Close()
		a.NewError(ctx, E.Cause(hErr, "process connection from ", metadata.Source))
	}
}
