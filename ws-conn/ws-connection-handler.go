package ws_conn

import (
	"fmt"
	"github.com/gorilla/websocket"
	"log"
	"sync"
	"time"
)

const PingPeriod = 30 * time.Second

// Websocket connection handler is connection storage.
// For che-machine-exec it used to manage connections with exec input/output.
type ConnectionHandler interface {
	// Add new websocket connection.
	ReadConnection(wsConn *websocket.Conn, inputChan chan []byte)
	// Send data to the client websocket connections.
	WriteDataToWsConnections(data []byte)

	// Remove websocket connection.
	removeConnection(wsConn *websocket.Conn)
	// Read data from websocket connection.
	readDataFromConnections(inputChan chan []byte, wsConn *websocket.Conn)
	// Send ping message to the websocket connection
	sendPingMessage(wsConn *websocket.Conn)
}

// Connection handler implementation.
type ConnectionHandlerImpl struct {
	wsConnsLock *sync.Mutex
	wsConns     []*websocket.Conn
}

// Create new implementation connection handler.
func NewConnHandler() *ConnectionHandlerImpl {
	return &ConnectionHandlerImpl{
		wsConnsLock: &sync.Mutex{},
		wsConns:     make([]*websocket.Conn, 0),
	}
}

// Add new connection to handler.
func (handler *ConnectionHandlerImpl) ReadConnection(wsConn *websocket.Conn, inputChan chan []byte) {
	defer handler.wsConnsLock.Unlock()
	handler.wsConnsLock.Lock()

	handler.wsConns = append(handler.wsConns, wsConn)

	go handler.readDataFromConnections(inputChan, wsConn)
	go handler.sendPingMessage(wsConn)
}

// Remove connection form handler.
func (handler *ConnectionHandlerImpl) removeConnection(wsConn *websocket.Conn) {
	defer handler.wsConnsLock.Unlock()
	handler.wsConnsLock.Lock()

	for index, wsConnElem := range handler.wsConns {
		if wsConnElem == wsConn {
			handler.wsConns = append(handler.wsConns[:index], handler.wsConns[index+1:]...)
		}
	}
}

// Write data to the all connections managed by handler.
func (handler *ConnectionHandlerImpl) WriteDataToWsConnections(data []byte) {
	defer handler.wsConnsLock.Unlock()
	handler.wsConnsLock.Lock()

	for _, wsConn := range handler.wsConns {
		if err := wsConn.WriteMessage(websocket.TextMessage, data); err != nil {
			fmt.Printf("failed to write to ws-conn message. Cause: %v", err)
			handler.removeConnection(wsConn)
		}
	}
}

// Read data from connection.
func (handler *ConnectionHandlerImpl) readDataFromConnections(inputChan chan []byte, wsConn *websocket.Conn) {
	defer handler.removeConnection(wsConn)

	for {
		msgType, wsBytes, err := wsConn.ReadMessage()
		if err != nil {
			log.Printf("failed to read ws-conn message. Cause: %v", err)
			return
		}

		if msgType != websocket.TextMessage {
			continue
		}

		inputChan <- wsBytes
	}
}

// Send ping message to the connection client.
func (*ConnectionHandlerImpl) sendPingMessage(wsConn *websocket.Conn) {
	ticker := time.NewTicker(PingPeriod)
	defer ticker.Stop()

	for range ticker.C {
		err := wsConn.WriteMessage(websocket.PingMessage, []byte{})
		if err != nil {
			if !IsNormalWSError(err) {
				log.Printf("Error occurs on sending ping message to ws-conn. %v", err)
			}
			return
		}
	}
}
