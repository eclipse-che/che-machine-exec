module github.com/eclipse/che-machine-exec

go 1.14

replace (
	cloud.google.com/go => cloud.google.com/go v0.54.0
	cloud.google.com/go/bigquery => cloud.google.com/go/bigquery v1.4.0
	cloud.google.com/go/datastore => cloud.google.com/go/datastore v1.1.0
	cloud.google.com/go/pubsub => cloud.google.com/go/pubsub v1.2.0
	cloud.google.com/go/storage => cloud.google.com/go/storage v1.6.0
	dmitri.shuralyov.com/gpu/mtl => dmitri.shuralyov.com/gpu/mtl v0.0.0-20190408044501-666a987793e9
	github.com/Azure/go-autorest => github.com/Azure/go-autorest v14.2.0+incompatible

	github.com/Azure/go-autorest/autorest => github.com/Azure/go-autorest/autorest v0.11.12
	github.com/Azure/go-autorest/autorest/adal => github.com/Azure/go-autorest/autorest/adal v0.9.5
	github.com/Azure/go-autorest/autorest/date => github.com/Azure/go-autorest/autorest/date v0.3.0
	github.com/Azure/go-autorest/autorest/mocks => github.com/Azure/go-autorest/autorest/mocks v0.4.1
	github.com/Azure/go-autorest/logger => github.com/Azure/go-autorest/logger v0.2.0
	github.com/Azure/go-autorest/tracing => github.com/Azure/go-autorest/tracing v0.6.0
	github.com/BurntSushi/toml => github.com/BurntSushi/toml v0.3.1

	github.com/BurntSushi/xgb => github.com/BurntSushi/xgb v0.0.0-20160522181843-27f122750802
	github.com/NYTimes/gziphandler => github.com/NYTimes/gziphandler v0.0.0-20170623195520-56545f4a5d46
	github.com/PuerkitoBio/purell => github.com/PuerkitoBio/purell v1.1.1
	github.com/PuerkitoBio/urlesc => github.com/PuerkitoBio/urlesc v0.0.0-20170810143723-de5bf2ad4578
	github.com/asaskevich/govalidator => github.com/asaskevich/govalidator v0.0.0-20190424111038-f61b66f89f4a
	github.com/census-instrumentation/opencensus-proto => github.com/census-instrumentation/opencensus-proto v0.2.1
	github.com/chzyer/logex => github.com/chzyer/logex v1.1.10
	github.com/chzyer/readline => github.com/chzyer/readline v0.0.0-20180603132655-2972be24d48e
	github.com/chzyer/test => github.com/chzyer/test v0.0.0-20180213035817-a1ea475d72b1
	github.com/client9/misspell => github.com/client9/misspell v0.3.4
	github.com/creack/pty => github.com/creack/pty v1.1.9
	github.com/davecgh/go-spew => github.com/davecgh/go-spew v1.1.1
	github.com/docopt/docopt-go => github.com/docopt/docopt-go v0.0.0-20180111231733-ee0de3bc6815
	github.com/eclipse/che-go-jsonrpc => github.com/eclipse/che-go-jsonrpc v0.0.0-20200317130110-931966b891fe
	github.com/elazarl/goproxy => github.com/elazarl/goproxy v0.0.0-20180725130230-947c36da3153
	github.com/emicklei/go-restful => github.com/emicklei/go-restful v0.0.0-20170410110728-ff4f55a20633
	github.com/envoyproxy/go-control-plane => github.com/envoyproxy/go-control-plane v0.9.1-0.20191026205805-5f8ba28d4473
	github.com/envoyproxy/protoc-gen-validate => github.com/envoyproxy/protoc-gen-validate v0.1.0
	github.com/evanphx/json-patch => github.com/evanphx/json-patch v4.9.0+incompatible
	github.com/form3tech-oss/jwt-go => github.com/form3tech-oss/jwt-go v3.2.2+incompatible
	github.com/fsnotify/fsnotify => github.com/fsnotify/fsnotify v1.4.7
	github.com/gin-contrib/sse => github.com/gin-contrib/sse v0.1.0
	github.com/gin-gonic/gin => github.com/gin-gonic/gin v1.6.3
	github.com/go-gl/glfw/v3.3/glfw => github.com/go-gl/glfw/v3.3/glfw v0.0.0-20200222043503-6f7a984d4dc4
	github.com/go-logr/logr => github.com/go-logr/logr v0.4.0
	github.com/go-openapi/jsonpointer => github.com/go-openapi/jsonpointer v0.19.3
	github.com/go-openapi/jsonreference => github.com/go-openapi/jsonreference v0.19.3
	github.com/go-openapi/spec => github.com/go-openapi/spec v0.19.3
	github.com/go-openapi/swag => github.com/go-openapi/swag v0.19.5

	github.com/go-playground/assert/v2 => github.com/go-playground/assert/v2 v2.0.1
	github.com/go-playground/locales => github.com/go-playground/locales v0.13.0
	github.com/go-playground/universal-translator => github.com/go-playground/universal-translator v0.17.0
	github.com/go-playground/validator/v10 => github.com/go-playground/validator/v10 v10.2.0
	github.com/gogo/protobuf => github.com/gogo/protobuf v1.3.2
	github.com/golang/glog => github.com/golang/glog v0.0.0-20160126235308-23def4e6c14b
	github.com/golang/groupcache => github.com/golang/groupcache v0.0.0-20200121045136-8c9f03a8e57e
	github.com/golang/mock => github.com/golang/mock v1.4.1
	github.com/golang/protobuf => github.com/golang/protobuf v1.4.3
	github.com/google/btree => github.com/google/btree v1.0.0
	github.com/google/go-cmp => github.com/google/go-cmp v0.5.2
	github.com/google/gofuzz => github.com/google/gofuzz v1.1.0
	github.com/google/martian => github.com/google/martian v2.1.0+incompatible
	github.com/google/pprof => github.com/google/pprof v0.0.0-20200229191704-1ebb73c60ed3
	github.com/google/renameio => github.com/google/renameio v0.1.0

	github.com/google/uuid => github.com/google/uuid v1.1.2

	github.com/googleapis/gax-go/v2 => github.com/googleapis/gax-go/v2 v2.0.5
	github.com/googleapis/gnostic => github.com/googleapis/gnostic v0.4.1
	github.com/gorilla/websocket => github.com/gorilla/websocket v1.4.2
	github.com/gregjones/httpcache => github.com/gregjones/httpcache v0.0.0-20180305231024-9cad4c3443a7

	github.com/hashicorp/golang-lru => github.com/hashicorp/golang-lru v0.5.1
	github.com/hpcloud/tail => github.com/hpcloud/tail v1.0.0
	github.com/ianlancetaylor/demangle => github.com/ianlancetaylor/demangle v0.0.0-20181102032728-5e5cf60278f6
	github.com/imdario/mergo => github.com/imdario/mergo v0.3.5
	github.com/json-iterator/go => github.com/json-iterator/go v1.1.10
	github.com/jstemmer/go-junit-report => github.com/jstemmer/go-junit-report v0.9.1
	github.com/kisielk/errcheck => github.com/kisielk/errcheck v1.5.0
	github.com/kisielk/gotool => github.com/kisielk/gotool v1.0.0
	github.com/kr/pretty => github.com/kr/pretty v0.2.0

	github.com/kr/pty => github.com/kr/pty v1.1.5
	github.com/kr/text => github.com/kr/text v0.2.0

	github.com/leodido/go-urn => github.com/leodido/go-urn v1.2.0
	github.com/mailru/easyjson => github.com/mailru/easyjson v0.0.0-20190626092158-b2ccc519800e
	github.com/mattn/go-isatty => github.com/mattn/go-isatty v0.0.12

	github.com/mitchellh/mapstructure => github.com/mitchellh/mapstructure v1.1.2

	github.com/moby/spdystream => github.com/moby/spdystream v0.2.0
	github.com/modern-go/concurrent => github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd
	github.com/modern-go/reflect2 => github.com/modern-go/reflect2 v1.0.1
	github.com/munnerz/goautoneg => github.com/munnerz/goautoneg v0.0.0-20120707110453-a547fc61f48d
	github.com/mxk/go-flowrate => github.com/mxk/go-flowrate v0.0.0-20140419014527-cca7078d478f

	github.com/niemeyer/pretty => github.com/niemeyer/pretty v0.0.0-20200227124842-a10e7caefd8e

	github.com/onsi/ginkgo => github.com/onsi/ginkgo v1.11.0

	github.com/onsi/gomega => github.com/onsi/gomega v1.7.0
	github.com/peterbourgon/diskv => github.com/peterbourgon/diskv v2.0.1+incompatible

	github.com/pkg/errors => github.com/pkg/errors v0.9.1

	github.com/pmezard/go-difflib => github.com/pmezard/go-difflib v1.0.0
	github.com/prometheus/client_model => github.com/prometheus/client_model v0.0.0-20190812154241-14fe0d1b01d4
	github.com/rogpeppe/go-internal => github.com/rogpeppe/go-internal v1.3.0

	github.com/sirupsen/logrus => github.com/sirupsen/logrus v1.8.1
	github.com/spf13/afero => github.com/spf13/afero v1.2.2
	github.com/spf13/pflag => github.com/spf13/pflag v1.0.5
	github.com/stretchr/objx => github.com/stretchr/objx v0.2.0
	github.com/stretchr/testify => github.com/stretchr/testify v1.6.1

	github.com/ugorji/go => github.com/ugorji/go v1.1.7

	github.com/ugorji/go/codec => github.com/ugorji/go/codec v1.1.7

	github.com/yuin/goldmark => github.com/yuin/goldmark v1.2.1
	go.opencensus.io => go.opencensus.io v0.22.3
	golang.org/x/crypto => golang.org/x/crypto v0.0.0-20210220033148-5ea612d1eb83
	golang.org/x/exp => golang.org/x/exp v0.0.0-20200224162631-6cc2880d07d6

	golang.org/x/image => golang.org/x/image v0.0.0-20190802002840-cff245a6509b
	golang.org/x/lint => golang.org/x/lint v0.0.0-20200302205851-738671d3881b

	golang.org/x/mobile => golang.org/x/mobile v0.0.0-20190719004257-d2bd2a29d028

	golang.org/x/mod => golang.org/x/mod v0.3.0

	golang.org/x/net => golang.org/x/net v0.0.0-20210224082022-3d97a244fca7
	golang.org/x/oauth2 => golang.org/x/oauth2 v0.0.0-20200107190931-bf48bf16ab8d
	golang.org/x/sync => golang.org/x/sync v0.0.0-20201020160332-67f06af15bc9
	golang.org/x/sys => golang.org/x/sys v0.0.0-20210225134936-a50acf3fe073
	golang.org/x/term => golang.org/x/term v0.0.0-20210220032956-6a3ed077a48d
	golang.org/x/text => golang.org/x/text v0.3.4
	golang.org/x/time => golang.org/x/time v0.0.0-20210220033141-f8bda1e9f3ba
	golang.org/x/tools => golang.org/x/tools v0.0.0-20210106214847-113979e3529a
	golang.org/x/xerrors => golang.org/x/xerrors v0.0.0-20200804184101-5ec99f83aff1
	google.golang.org/api => google.golang.org/api v0.20.0
	google.golang.org/appengine => google.golang.org/appengine v1.6.5
	google.golang.org/genproto => google.golang.org/genproto v0.0.0-20200526211855-cb27e3aa2013
	google.golang.org/grpc => google.golang.org/grpc v1.27.1
	google.golang.org/protobuf => google.golang.org/protobuf v1.25.0
	gopkg.in/check.v1 => gopkg.in/check.v1 v1.0.0-20200227125254-8fa46927fb4f
	gopkg.in/errgo.v2 => gopkg.in/errgo.v2 v2.1.0
	gopkg.in/fsnotify.v1 => gopkg.in/fsnotify.v1 v1.4.7
	gopkg.in/inf.v0 => gopkg.in/inf.v0 v0.9.1
	gopkg.in/tomb.v1 => gopkg.in/tomb.v1 v1.0.0-20141024135613-dd632973f1e7
	gopkg.in/yaml.v2 => gopkg.in/yaml.v2 v2.4.0
	gopkg.in/yaml.v3 => gopkg.in/yaml.v3 v3.0.0-20200313102051-9f266ea9e77c
	honnef.co/go/tools => honnef.co/go/tools v0.0.1-2020.1.3

	k8s.io/api => k8s.io/api v0.0.0-20210129201028-cfb031d9922e
	k8s.io/apimachinery => k8s.io/apimachinery v0.21.0-beta.0
	k8s.io/client-go => k8s.io/client-go v0.21.0-beta.0
	k8s.io/gengo => k8s.io/gengo v0.0.0-20200413195148-3a45101e95ac
	k8s.io/klog/v2 => k8s.io/klog/v2 v2.8.0
	k8s.io/kube-openapi => k8s.io/kube-openapi v0.0.0-20210305001622-591a79e4bda7
	k8s.io/utils => k8s.io/utils v0.0.0-20201110183641-67b214c5f920
	rsc.io/quote/v3 => rsc.io/quote/v3 v3.1.0
	rsc.io/sampler => rsc.io/sampler v1.3.0
	sigs.k8s.io/structured-merge-diff/v4 => sigs.k8s.io/structured-merge-diff/v4 v4.1.0
	sigs.k8s.io/yaml => sigs.k8s.io/yaml v1.2.0
)

require (
	github.com/eclipse/che-go-jsonrpc v0.0.0-00010101000000-000000000000
	github.com/gin-gonic/gin v1.6.3
	github.com/gorilla/websocket v1.4.2
	github.com/niemeyer/pretty v0.0.0-00010101000000-000000000000 // indirect
	github.com/pkg/errors v0.9.1
	github.com/sirupsen/logrus v1.8.1
	github.com/stretchr/testify v1.6.1
	gopkg.in/yaml.v2 v2.4.0
	k8s.io/api v0.21.0-beta.0
	k8s.io/apimachinery v0.21.0-beta.0
	k8s.io/client-go v0.21.0-beta.0
)
