package events

import (
	"github.com/eclipse/che-go-jsonrpc"
	"github.com/eclipse/che-go-jsonrpc/event"
	"log"
)

// Event bus to send events with information about execs to the clients.
var EventBus = event.NewBus()

// Exec Event consumer to send exec events to the clients with help json-rpc tunnel.
// INFO: Tunnel it's one of the active json-rpc connection.
type ExecEventConsumer struct {
	event.Consumer
	Tunnel *jsonrpc.Tunnel
}

// Send event to the client with help json-rpc tunnel.
func (execConsumer *ExecEventConsumer) Accept(event event.E) {
	if !execConsumer.Tunnel.IsClosed() {
		if err := execConsumer.Tunnel.Notify(event.Type(), event); err != nil {
			log.Println("Unable to send event to the tunnel: ", execConsumer.Tunnel.ID(), "Cause: ", err.Error())
		}
	}
}
