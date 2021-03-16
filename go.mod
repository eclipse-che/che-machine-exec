module github.com/eclipse/che-machine-exec

go 1.14

replace (
	github.com/eclipse/che-go-jsonrpc => github.com/eclipse/che-go-jsonrpc v0.0.0-20200317130110-931966b891fe
	k8s.io/client-go => k8s.io/client-go v0.0.0-20210313030403-f6ce18ae578c
)

require (
	github.com/eclipse/che-go-jsonrpc v0.0.0-00010101000000-000000000000
	github.com/gin-gonic/gin v1.6.3
	github.com/gorilla/websocket v1.4.2
	github.com/pkg/errors v0.9.1
	github.com/sirupsen/logrus v1.8.1
	github.com/stretchr/testify v1.6.1
	gopkg.in/yaml.v2 v2.4.0
	k8s.io/api v0.0.0-20210313025757-51a1c5553d68
	k8s.io/apimachinery v0.0.0-20210313025227-57f2a0733447
	k8s.io/client-go v0.0.0-20210313030403-f6ce18ae578c
)
