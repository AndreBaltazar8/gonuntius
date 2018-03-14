package gonuntius

import (
	"bytes"
	"errors"
	"fmt"
	"math/rand"
	"net"
	"sync"

	"github.com/AndreBaltazar8/autorpc"
)

type Connection interface {
	Register(appID []byte, publicID []byte, registrationKey []byte, fn func(secreyKey []byte, err error))
	Authenticate(appID []byte, publicID []byte, secretKey []byte, fn func(error))
	ConnectTo(publicID []byte, fn func(RemoteConnection, error))
	OnReady(fn func())
	OnIncomingConnection(fn func(IncomingConnection))
}

type pendingConn struct {
	fn       func(RemoteConnection, error)
	remoteID []byte
}

type connImpl struct {
	conn           net.Conn
	connRemote     *nuntiusServerAPI
	appID          []byte
	publicID       []byte
	ready          bool
	readyFns       []func()
	incConnFns     []func(IncomingConnection)
	acceptingConn  *incomingConnection
	isStreaming    bool
	acceptReturned bool
	startStreaming func()
	pendingConn    sync.Map
	rpcConn        autorpc.Connection
}

func (conn *connImpl) Register(appID []byte, publicID []byte, registrationKey []byte, fn func(secreyKey []byte, err error)) {
	if !conn.ready {
		return
	}

	conn.connRemote.Register(appID, registrationKey, publicID, fn)
}

func (conn *connImpl) Authenticate(appID []byte, publicID []byte, secretKey []byte, fn func(error)) {
	if !conn.ready {
		return
	}

	conn.connRemote.Authenticate(appID, publicID, secretKey, fn)
	conn.appID = publicID
	conn.publicID = publicID
}

func (conn *connImpl) getPendingConnection(id int) (*pendingConn, bool) {
	val, ok := conn.pendingConn.Load(id)
	if !ok {
		return nil, false
	}
	if v, ok := val.(*pendingConn); ok {
		return v, true
	}
	panic("stored pending connection has wrong type")
}

func (conn *connImpl) ConnectTo(publicID []byte, fn func(RemoteConnection, error)) {
	pendingConn := &pendingConn{fn, publicID}
	var pendingID int
	for {
		pendingID = rand.Int()
		if _, exists := conn.pendingConn.LoadOrStore(pendingID, pendingConn); !exists {
			conn.connRemote.ConnectTo(publicID, pendingID, func(err error) {
				if err != nil {
					conn.pendingConn.Delete(pendingID)
					fn(nil, err)
				}
			})
			return
		}
	}
}

func (conn *connImpl) OnReady(fn func()) {
	conn.readyFns = append(conn.readyFns, fn)

	if conn.ready {
		fn()
		return
	}
}

func (conn *connImpl) OnIncomingConnection(fn func(IncomingConnection)) {
	conn.incConnFns = append(conn.incConnFns, fn)
}

func (conn *connImpl) acceptConnection(incConn *incomingConnection) {
	if conn.acceptingConn != nil {
		return
	}

	conn.acceptingConn = incConn
	conn.connRemote.AcceptConnection(incConn.connID, func(err error) {
		if err != nil {
			incConn.acceptCallback(nil, err)
		}

		conn.acceptReturned = true
		if conn.startStreaming != nil {
			conn.isStreaming = true
			conn.rpcConn.StopHandling()
		}
	})
}

const (
	gatewayAddress = "127.0.0.1:4444"
)

type nuntiusServerAPI struct {
	Version          func(byte, func(error))
	Register         func([]byte, []byte, []byte, func([]byte, error))
	Authenticate     func([]byte, []byte, []byte, func(error))
	ConnectTo        func([]byte, int, func(error))
	RejectConnection func([]byte, func(error))
	AcceptConnection func([]byte, func(error))
}

type nuntiusClientAPI struct {
}

func (api *nuntiusClientAPI) IncomingConnection(connID []byte, pendingID int, remoteID []byte, remote *nuntiusServerAPI, conn *connImpl) {
	incConn := incomingConnection{remoteID: remoteID, connID: connID, remoteAPI: remote}
	fmt.Println("incoming conn remoteid", string(remoteID), pendingID)
	if bytes.Compare(remoteID, conn.publicID) == 0 {
		if pendingConn, ok := conn.getPendingConnection(pendingID); ok {
			conn.pendingConn.Delete(pendingID)
			incConn.remoteID = pendingConn.remoteID
			incConn.Accept(pendingConn.fn)
		} else {
			incConn.Reject()
			pendingConn.fn(nil, errors.New("connection was rejected locally, because it could not be found in pending connections"))
		}
		return
	}

	for _, fn := range conn.incConnFns {
		fn(&incConn)
	}

	if !incConn.IsHandled() {
		incConn.Reject()
	}
}

func (api *nuntiusClientAPI) InitializeConnection(connID []byte, conn *connImpl) {
	pendingConn := conn.acceptingConn
	if bytes.Compare(pendingConn.connID, connID) == 0 {
		if conn.acceptReturned {
			conn.isStreaming = true
			conn.rpcConn.StopHandling()
		}

		conn.startStreaming = func() {
			remoteConn := remoteConnection{conn, pendingConn.remoteID}
			conn.acceptingConn.acceptCallback(&remoteConn, nil)
		}
	}
}

func (api *nuntiusClientAPI) ErrorConnection(connID []byte, err string, conn *connImpl) {
	pendingConn := conn.acceptingConn
	if bytes.Compare(pendingConn.connID, connID) == 0 {
		conn.acceptingConn.acceptCallback(nil, errors.New(err))
	}
}

func NewConnection() (Connection, error) {
	conn, err := net.Dial("tcp", gatewayAddress)
	if err != nil {
		return nil, err
	}

	service := autorpc.NewServiceBuilder(&nuntiusClientAPI{}).EachConnectionAssign(&connImpl{}, nil).UseRemote(nuntiusServerAPI{}).Build()

	nuntiusConn := connImpl{
		conn: conn,
	}

	go func() {
		err := service.HandleConnection(conn, func(connection autorpc.Connection) {
			remote, err := connection.GetValue(nuntiusServerAPI{})
			if err != nil {
				panic("could not get remote api for client")
			}

			if remote, ok := remote.(*nuntiusServerAPI); ok {
				remote.Version(2, func(err error) {
					if err != nil {
						fmt.Println(err)
						return
					}

					nuntiusConn.ready = true
					for _, fn := range nuntiusConn.readyFns {
						fn()
					}
				})
				nuntiusConn.connRemote = remote
			} else {
				panic("unknown type returned for remote api")
			}

			err = connection.AssignValue(&nuntiusConn)
			if err != nil {
				panic(fmt.Sprint("could not assign client:", err))
			}

			nuntiusConn.rpcConn = connection
		})

		if err != nil {
			return
		}

		if nuntiusConn.startStreaming != nil {
			nuntiusConn.startStreaming()
		} else {
			panic("should have started streaming, but streaming is nil")
		}
	}()

	return &nuntiusConn, nil
}
