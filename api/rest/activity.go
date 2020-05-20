package rest

import (
	"github.com/gin-gonic/gin"
	"net/http"
)

func HandleActivityTick(c *gin.Context) {
	// at this point, it's just stub handler that does nothing
	// but a bit later ActivityManager will appear and register the latest activity
	// to post pone workspace stopping by idle timeout
	c.Writer.WriteHeader(http.StatusNoContent)
	return
}
