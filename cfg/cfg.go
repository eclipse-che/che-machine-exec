//
// Copyright (c) 2019-2021 Red Hat, Inc.
// This program and the accompanying materials are made
// available under the terms of the Eclipse Public License 2.0
// which is available at https://www.eclipse.org/legal/epl-2.0/
//
// SPDX-License-Identifier: EPL-2.0
//
// Contributors:
//   Red Hat, Inc. - initial API and implementation
//

package cfg

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
)

var (
	// URL che-machine-exec api server url
	URL string
	// UseBearerToken - flag to enable/disable using bearer token to avoid users impersonation while accessing to k8s API.
	// if enabled - authenticated user ID must be configured
	UseBearerToken bool
	// AuthenticatedUserID is a user's ID who is authenticated to use API. Is ignored if useBearerToken is disabled
	AuthenticatedUserID string
	// NoAuth explicitly disables authentication. It is mutually exclusive with
	// UseBearerToken. One of the two must be chosen explicitly, otherwise the
	// server refuses to start (see Parse), so there is no silent unauthenticated
	// default. Running with NoAuth and no AllowedOrigins leaves the API open to
	// any client that can reach the port, so it must only be used on a loopback
	// or otherwise trusted network (as che-code does over 127.0.0.1).
	NoAuth bool

	// IdleTimeout is a inactivity period after which workspace should be stopped
	// Default value is 30 minutes
	IdleTimeout time.Duration
	// StopRetryPeriod is a period after which workspace should be tried to stop if the previous try failed
	// Default value is 10 seconds
	StopRetryPeriod time.Duration
	// RunTimeout is the maximum duration a workspace can be running before it is stopped
	// Default value is -1, which means - no maximium duration
	RunTimeout time.Duration

	// UseTLS flag to enable/disable serving TLS
	UseTLS bool

	// PodSelector set of labels to be used as selector for getting workspace pod.
	// Default value is che.workspace_id=${CHE_WORKSPACE_ID}
	PodSelector string

	// AllowedOrigins is an optional list of browser origins (scheme://host[:port])
	// that are allowed to open the WebSocket endpoints. It is a defense-in-depth
	// measure against Cross-Site WebSocket Hijacking. A request with no Origin
	// header (non-browser clients) or a same-origin request is always allowed, so
	// leaving this empty keeps the current behavior for the supported deployments
	// (che-code connects over loopback without an Origin header).
	AllowedOrigins []string

	// allowedOriginsFlag backs both the ALLOWED_ORIGINS env var (default) and the
	// -allowed-origins flag. Its value is copied into AllowedOrigins by Parse(),
	// after flag.Parse() has run.
	allowedOriginsFlag stringSliceFlag
)

// stringSliceFlag is a flag.Value that parses a comma-separated list of strings.
type stringSliceFlag []string

func (s *stringSliceFlag) String() string {
	return strings.Join(*s, ",")
}

func (s *stringSliceFlag) Set(value string) error {
	*s = parseAllowedOrigins(value)
	return nil
}

// parseAllowedOrigins splits a comma-separated list, trimming whitespace and
// dropping empty entries. A blank input yields an empty (nil) slice.
func parseAllowedOrigins(value string) []string {
	var origins []string
	for _, origin := range strings.Split(value, ",") {
		if trimmed := strings.TrimSpace(origin); trimmed != "" {
			origins = append(origins, trimmed)
		}
	}
	return origins
}

func init() {
	defaultURLValue := ":4444"
	urlEnvValue, isFound := os.LookupEnv("API_URL")
	if isFound && len(urlEnvValue) > 0 {
		defaultURLValue = urlEnvValue
	}
	flag.StringVar(&URL, "url", defaultURLValue, "Host:Port address.")

	defaultUseTokenValue := false
	useTokenEnv := "USE_BEARER_TOKEN"
	useTokenEnvValue, isFound := os.LookupEnv(useTokenEnv)
	if isFound && len(useTokenEnvValue) > 0 {
		if v, err := strconv.ParseBool(useTokenEnvValue); err == nil {
			defaultUseTokenValue = v
		} else {
			logrus.Errorf("Invalid value '%s' for env variable key '%s'. Value should be boolean", useTokenEnvValue, useTokenEnv)
		}
	}
	flag.BoolVar(&UseBearerToken, "use-bearer-token", defaultUseTokenValue, "to avoid users impersonation while accessing to k8s API. When enabled - authenticated user id must be configured")

	defaultNoAuthValue := false
	noAuthEnv := "NO_AUTH"
	noAuthEnvValue, isFound := os.LookupEnv(noAuthEnv)
	if isFound && len(noAuthEnvValue) > 0 {
		if v, err := strconv.ParseBool(noAuthEnvValue); err == nil {
			defaultNoAuthValue = v
		} else {
			logrus.Errorf("Invalid value '%s' for env variable key '%s'. Value should be boolean", noAuthEnvValue, noAuthEnv)
		}
	}
	flag.BoolVar(&NoAuth, "no-auth", defaultNoAuthValue, "Explicitly disable authentication. Mutually exclusive with -use-bearer-token; one of the two must be set. Without an allowed-origins list the API is open to any client that can reach the port, so -no-auth must only be used on a loopback or otherwise trusted network.")

	defaultAuthenticatedUserID := ""
	authenticatedUserID, isFound := os.LookupEnv("AUTHENTICATED_USER_ID")
	if isFound {
		defaultAuthenticatedUserID = authenticatedUserID
	}
	flag.StringVar(&AuthenticatedUserID, "authenticated-user-id", defaultAuthenticatedUserID, "OpenShift user's ID that should has access to API. Is used only if useBearerToken is configured")

	const defaultIdleTimeout = 1800 * time.Second
	idleTimeout := defaultIdleTimeout
	idleTimeoutEnv := "SECONDS_OF_DW_INACTIVITY_BEFORE_IDLING"
	idleTimeoutEnvValue, isFound := os.LookupEnv(idleTimeoutEnv)
	if isFound && len(idleTimeoutEnvValue) > 0 {
		if v, err := strconv.ParseInt(idleTimeoutEnvValue, 10, 32); err == nil {
			idleTimeout = time.Duration(v) * time.Second
		} else {
			logrus.Errorf("Invalid value '%s' for env variable key '%s'. Value should be an integer", idleTimeoutEnvValue, idleTimeoutEnv)
		}
	}
	flag.DurationVar(&IdleTimeout, "idle-timeout", idleTimeout, "IdleTimeout is a inactivity period after which workspace should be stopped. By default, IdleTimeout is set to 30m. To disable IdleTimeout, set to -1. Examples: -1, 30s, 15m, 1h")

	const defaultRunTimeout = -1 * time.Nanosecond
	runtimeout := defaultRunTimeout
	runTimeoutEnv := "SECONDS_OF_DW_RUN_BEFORE_IDLING"
	runTimeoutEnvValue, isFound := os.LookupEnv(runTimeoutEnv)
	if isFound && len(runTimeoutEnvValue) > 0 {
		if v, err := strconv.ParseInt(runTimeoutEnvValue, 10, 32); err == nil {
			runtimeout = time.Duration(v) * time.Second
		} else {
			logrus.Errorf("Invalid value '%s' for env variable key '%s'. Value should be an integer", runTimeoutEnvValue, runTimeoutEnv)
		}
	}
	flag.DurationVar(&RunTimeout, "run-timeout", runtimeout, "RunTimeout is the maximum duration a workspace can run. After this period, the workspace will be stopped. Examples: -1, 30s, 15m, 1h")

	flag.DurationVar(&StopRetryPeriod, "stop-retry-period", 10*time.Second, "StopRetryPeriod is a period after which workspace should be tried to stop if the previous try failed. Examples: 30s")

	flag.BoolVar(&UseTLS, "use-tls", false, "Serve content via TLS")

	defaultPodSelector, isFound := os.LookupEnv("POD_SELECTOR")
	if !isFound {
		workspaceID := os.Getenv("DEVWORKSPACE_ID")
		if workspaceID != "" {
			defaultPodSelector = fmt.Sprintf("controller.devfile.io/devworkspace_id=%s", workspaceID)
		} else {
			workspaceID = os.Getenv("CHE_WORKSPACE_ID")
			if workspaceID != "" {
				defaultPodSelector = fmt.Sprintf("che.workspace_id=%s", workspaceID)
			}
		}
	}
	flag.StringVar(&PodSelector, "pod-selector", defaultPodSelector, "Selector that is used to find workspace pod. Default value is `che.workspace_id=${CHE_WORKSPACE_ID}` or controller.devfile.io/devworkspace_id={DEVWORKSPACE_ID} if che env var is not defined")

	allowedOriginsFlag = parseAllowedOrigins(os.Getenv("ALLOWED_ORIGINS"))
	flag.Var(&allowedOriginsFlag, "allowed-origins", "Comma-separated list of browser origins (scheme://host[:port]) allowed to open the WebSocket endpoints. Requests with no Origin header or same-origin requests are always allowed. Defaults to the ALLOWED_ORIGINS env var.")

	setLogLevel()
}

func setLogLevel() {
	logLevel, isFound := os.LookupEnv("LOG_LEVEL")
	if isFound && len(logLevel) > 0 {
		parsedLevel, err := logrus.ParseLevel(logLevel)
		if err == nil {
			logrus.SetLevel(parsedLevel)
			logrus.Infof("Configured '%s' log level is applied", logLevel)
		} else {
			logrus.Errorf("Failed to parse log level `%s`. Possible values: panic, fatal, error, warn, info, debug. Default 'info' is applied", logLevel)
			logrus.SetLevel(logrus.InfoLevel)
		}
	} else {
		logrus.Infof("Default 'info' log level is applied")
		logrus.SetLevel(logrus.InfoLevel)
	}
}

// Parse application arguments
func Parse() {
	flag.Parse()

	AllowedOrigins = allowedOriginsFlag

	validateAuthMode()

	if PodSelector == "" {
		logrus.Fatal("pod selector is required. Configure custom pod selector or che workspace/devworkspace id env var to activate defaults")
	}

	if StopRetryPeriod <= 0 {
		logrus.Fatalf("stop-retry-period must be greater than 0")
	}
}

// validateAuthMode enforces that an authentication mode is chosen explicitly, so
// che-machine-exec never starts in a silently unauthenticated state. Exactly one
// of -use-bearer-token or -no-auth must be set, otherwise the server refuses to
// start. When authentication is disabled it warns about the exposure risk.
func validateAuthMode() {
	if err := checkAuthMode(NoAuth, UseBearerToken); err != nil {
		logrus.Fatal(err)
	}

	if NoAuth {
		if len(AllowedOrigins) > 0 {
			logrus.Warnf("Authentication is DISABLED (-no-auth). Access is restricted to the configured origins (%s), which only blocks browsers; a direct client that omits the Origin header is not stopped. Do not rely on this alone on an untrusted network.", strings.Join(AllowedOrigins, ", "))
		} else {
			logrus.Warn("Authentication is DISABLED (-no-auth) and no allowed origins are configured. The API is open to any client that can reach the port. Only use this on a loopback or otherwise trusted network - do NOT expose this port, especially in production.")
		}
	}
}

// checkAuthMode returns an error when the authentication mode is not chosen
// explicitly. Exactly one of noAuth or useBearer must be set.
func checkAuthMode(noAuth, useBearer bool) error {
	switch {
	case noAuth && useBearer:
		return errors.New("-no-auth and -use-bearer-token are mutually exclusive: choose exactly one authentication mode")
	case !noAuth && !useBearer:
		return errors.New("no authentication mode selected: start che-machine-exec with -use-bearer-token (recommended) or, only on a loopback/trusted network, with -no-auth")
	}
	return nil
}

// Print configuration information
func Print() {
	logrus.Info("Exec containers configuration:")

	logrus.Infof("==> Debug level %s", logrus.GetLevel().String())
	logrus.Infof("==> Application url %s", URL)
	logrus.Infof("==> Use bearer token: %t", UseBearerToken)
	logrus.Infof("==> Authentication disabled (no-auth): %t", NoAuth)
	logrus.Infof("==> Pod selector: %s", PodSelector)
	if len(AllowedOrigins) > 0 {
		logrus.Infof("==> Allowed WebSocket origins: %s", strings.Join(AllowedOrigins, ", "))
	} else {
		logrus.Infof("==> Allowed WebSocket origins: <none configured; only same-origin and origin-less requests accepted>")
	}
	if UseBearerToken {
		logrus.Infof("==> Authenticated user ID: %s", AuthenticatedUserID)
	}
	if IdleTimeout > 0 {
		logrus.Infof("==> Idle timeout: %s", IdleTimeout)
	}
	if RunTimeout > 0 {
		logrus.Infof("==> Run timeout: %s", RunTimeout)
	}
	if IdleTimeout > 0 || RunTimeout > 0 {
		logrus.Infof("==> Stop retry period: %s", StopRetryPeriod)
	}
}
