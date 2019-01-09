package ws_conn

import (
	ws "github.com/gorilla/websocket"
)

// Return true if err is normal close connection error(connection close client). Return false otherwise.
// Note: In case if connection was close normally by client it's ok for us, we should not log error.
func IsNormalWSError(err error) bool {
	normalCloseCodes := []int{ws.CloseGoingAway, ws.CloseNormalClosure, ws.CloseNoStatusReceived}
	if closeErr, ok := err.(*ws.CloseError); ok {
		for _, code := range normalCloseCodes {
			if closeErr.Code == code {
				return true
			}
		}
	}
	return false
}
