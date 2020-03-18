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

var (
	url, staticPath string
)

type execContainersConfig struct {
	staticPath string
	url      string

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

func init() {
	flag.StringVar(&url, "url", ":4444", "Host:Port address.")
	flag.StringVar(&staticPath, "static", "", "/home/user/frontend - absolute path to folder with static resources.")
}

func Parse() {

}

func Print() {
	log.Print("Container exec configuration")
	// log.Printf("  Push endpoint: %s", PushStatusesEndpoint)
	// log.Printf("  Push logs endpoint: %s", PushLogsEndpoint)
	// log.Printf("  Auth enabled: %t", AuthEnabled)
	// if (SelfSignedCertificateFilePath != "") {
	// 	log.Printf("  Self signed certificate %s", SelfSignedCertificateFilePath)
	// }
	// log.Print("  Runtime ID:")
	// log.Printf("    Workspace: %s", RuntimeID.Workspace)
	// log.Printf("    Environment: %s", RuntimeID.Environment)
	// log.Printf("    OwnerId: %s", RuntimeID.OwnerId)
	// log.Printf("  Machine name: %s", MachineName)
	// log.Printf("  Installer timeout: %dseconds", InstallerTimeoutSec)
	// log.Printf("  Check servers period: %dseconds", CheckServersPeriodSec)
	// log.Printf("  Push logs endpoint reconnect period: %dseconds", LogsEndpointReconnectPeriodSec)
}