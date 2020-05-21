package rest

import (
	"github.com/eclipse/che-machine-exec/auth"
	restUtil "github.com/eclipse/che-machine-exec/common/rest"
	"github.com/gin-gonic/gin"
	"net/http"
)

func HandleActivityTick(c *gin.Context) {
	if auth.IsEnabled() {
		_, err := auth.Authenticate(c)
		if err != nil {
			restUtil.WriteErrorResponse(c, err)
			return
		}
	}

	// at this point, it's just stub handler that does nothing
	// but a bit later ActivityManager will appear and register the latest activity
	// to post pone workspace stopping by idle timeout
	c.Writer.WriteHeader(http.StatusNoContent)
	return
}
