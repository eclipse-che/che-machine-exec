//
// Copyright (c) 2012-2020 Red Hat, Inc.
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
	"flag"
	"os"
	"strconv"

	"github.com/sirupsen/logrus"
)

var (
	// URL che-machine-exec api server url
	URL string
	// StaticPath path to serve static resources
	StaticPath string
	// UseBearerToken - flag to enable/disable using bearer token to avoid users impersonation while accessing to k8s API.
	UseBearerToken bool
)

func init() {
	defaultURLValue := ":4444"
	urlEnvValue, isFound := os.LookupEnv("API_URL")
	if isFound && len(urlEnvValue) > 0 {
		defaultURLValue = urlEnvValue
	}
	flag.StringVar(&URL, "url", defaultURLValue, "Host:Port address.")

	defaultStaticPath := ""
	staticPathEnvValue, isFound := os.LookupEnv("STATIC_RESOURCES_PATH")
	if isFound && len(staticPathEnvValue) > 0 {
		defaultStaticPath = staticPathEnvValue
	}
	flag.StringVar(&StaticPath, "static", defaultStaticPath, "/home/user/frontend - absolute path to folder with static resources.")

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
	flag.BoolVar(&UseBearerToken, "use-bearer-token", defaultUseTokenValue, "to have access to the kubernetes api")

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
}

// Print configuration information
func Print() {
	logrus.Info("Exec containers configuration:")

	logrus.Infof("==> Debug level %s", logrus.GetLevel().String())
	logrus.Infof("==> Application url %s", URL)
	logrus.Infof("==> Absolute path to folder with static resources %s", StaticPath)
	logrus.Infof("==> Use bearer token: %t", UseBearerToken)
}
