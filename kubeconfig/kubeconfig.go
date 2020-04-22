//
// Copyright (c) 2019-2020 Red Hat, Inc.
// This program and the accompanying materials are made
// available under the terms of the Eclipse Public License 2.0
// which is available at https://www.eclipse.org/legal/epl-2.0/
//
// SPDX-License-Identifier: EPL-2.0
//
// Contributors:
//   Red Hat, Inc. - initial API and implementation
//
package kubeconfig

import (
	"io/ioutil"
	"net"
	"os"

	"github.com/eclipse/che-machine-exec/exec"
	"github.com/sirupsen/logrus"
	"gopkg.in/yaml.v2"
)

type KubeConfig struct {
	APIVersion     string     `yaml:"apiVersion"`
	Clusters       []Clusters `yaml:"clusters"`
	Users          []Users    `yaml:"users"`
	Contexts       []Contexts `yaml:"contexts"`
	CurrentContext string     `yaml:"current-context"`
	Kind           string     `yaml:"kind"`
}

type Clusters struct {
	Cluster ClusterInfo `yaml:"cluster"`
	Name    string      `yaml:"name"`
}

type ClusterInfo struct {
	CertificateAuthority string `yaml:"certificate-authority"`
	Server               string `yaml:"server"`
}

type Users struct {
	Name string `yaml:"name"`
	User User   `yaml:"user"`
}

type User struct {
	Token string `yaml:"token"`
}

type Contexts struct {
	Context Context `yaml:"context"`
	Name    string  `yaml:"name"`
}

type Context struct {
	Cluster   string `yaml:"cluster"`
	Namespace string `yaml:"namespace"`
	User      string `yaml:"user"`
}

func generateKubeConfig(token, server, namespace string) *KubeConfig {
	return &KubeConfig{
		APIVersion: "v1",
		Clusters: []Clusters{
			Clusters{
				Cluster: ClusterInfo{
					CertificateAuthority: "/var/run/secrets/kubernetes.io/serviceaccount/ca.crt",
					Server:               server,
				},
				Name: server,
			},
		},
		Users: []Users{
			Users{
				Name: "developer",
				User: User{
					Token: token,
				},
			},
		},
		Contexts: []Contexts{
			Contexts{
				Context: Context{
					Cluster:   server,
					Namespace: namespace,
					User:      "developer",
				},
				Name: "developer-context",
			},
		},
		CurrentContext: "developer-context",
		Kind:           "Config",
	}
}

// CreateKubeConfig creates a kubeconfig located at os.Getenv("KUBECONFIG") and puts in the
// values specific to the user IFF KUBECONFIG env variable is set
func CreateKubeConfig(token string) {
	kubeConfigLocation := os.Getenv("KUBECONFIG")
	if kubeConfigLocation == "" {
		return
	}

	host, port := os.Getenv("KUBERNETES_SERVICE_HOST"), os.Getenv("KUBERNETES_SERVICE_PORT")
	if host == "" || port == "" {
		return
	}

	namespace := exec.GetNamespace()

	server := "https://" + net.JoinHostPort(host, port)
	kubeconfig := generateKubeConfig(token, server, string(namespace))

	bytes, err := yaml.Marshal(&kubeconfig)
	if err != nil {
		logrus.Error("error: %v", err)
		return
	}

	err = ioutil.WriteFile(kubeConfigLocation, bytes, 0655)
	if err != nil {
		logrus.Error("error: %v", err)
		return
	}
}
