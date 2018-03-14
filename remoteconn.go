package gonuntius

import (
	"errors"
	"net"
	"time"
)

type RemoteConnection interface {
	Read(b []byte) (n int, err error)
	Write(b []byte) (n int, err error)
	Close() error
	SetDeadline(t time.Time) error
	SetReadDeadline(t time.Time) error
	SetWriteDeadline(t time.Time) error
	GetRemoteID() []byte
	LocalAddr() net.Addr
	RemoteAddr() net.Addr
}

type remoteConnection struct {
	conn     *connImpl
	remoteID []byte
}

func (conn *remoteConnection) Read(b []byte) (n int, err error) {
	return conn.conn.conn.Read(b)
}

func (conn *remoteConnection) Write(b []byte) (n int, err error) {
	return conn.conn.conn.Write(b)
}

func (conn *remoteConnection) Close() error {
	return conn.conn.conn.Close()
}

func (conn *remoteConnection) SetDeadline(t time.Time) error {
	return conn.conn.conn.SetDeadline(t)
}

func (conn *remoteConnection) SetReadDeadline(t time.Time) error {
	return conn.conn.conn.SetReadDeadline(t)
}

func (conn *remoteConnection) SetWriteDeadline(t time.Time) error {
	return conn.conn.conn.SetWriteDeadline(t)
}

func (conn *remoteConnection) GetRemoteID() []byte {
	return conn.remoteID
}

func (conn *remoteConnection) RemoteAddr() net.Addr {
	return nil
}

func (conn *remoteConnection) LocalAddr() net.Addr {
	return nil
}

type IncomingConnection interface {
	IsHandled() bool
	GetRemoteID() []byte
	Accept(func(RemoteConnection, error)) error
	Reject() error
}

type incomingConnection struct {
	isHandled      bool
	connID         []byte
	remoteID       []byte
	remoteAPI      *nuntiusServerAPI
	acceptCallback func(RemoteConnection, error)
}

func (conn *incomingConnection) IsHandled() bool {
	return conn.isHandled
}

func (conn *incomingConnection) GetRemoteID() []byte {
	return conn.remoteID
}

func (conn *incomingConnection) Accept(fn func(RemoteConnection, error)) error {
	if conn.isHandled {
		return errors.New("connection already handled")
	}

	conn.acceptCallback = fn
	conn.isHandled = true

	newConn, err := NewConnection()
	if err != nil {
		return err
	}

	if connImpl, ok := newConn.(*connImpl); ok {
		newConn.OnReady(func() {
			connImpl.acceptConnection(conn)
		})
	}

	return nil
}

func (conn *incomingConnection) Reject() error {
	if conn.isHandled {
		return errors.New("connection already handled")
	}
	conn.remoteAPI.RejectConnection(conn.connID, func(error) {})
	return nil
}
