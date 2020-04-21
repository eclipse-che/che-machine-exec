[![Master Build Status](https://ci.centos.org/buildStatus/icon?subject=master&job=devtools-che-machine-exec-build-master/)](https://ci.centos.org/job/devtools-che-machine-exec-build-master/)
[![Nightly Build Status](https://ci.centos.org/buildStatus/icon?subject=nightly&job=devtools-che-machine-exec-nightly)](https://ci.centos.org/job/devtools-che-machine-exec-nightly/)
[![Release Build Status](https://ci.centos.org/buildStatus/icon?subject=release&job=devtools-che-machine-exec-release/)](https://ci.centos.org/job/devtools-che-machine-exec-release/)

# Che machine exec

Golang server that creates machine-execs for Eclipse Che workspaces.

Used to spawn terminals or command processes.

Che machine exec uses json-rpc protocol to communicate with client.

## Build docker image

Build docker image with che-machine-exec manually:

```bash
docker build --no-cache -t eclipse/che-machine-exec .
```

## Test che-machine-exec on OpenShift

First, [build Eclipse Che Assembly](#build-eclipse-che-assembly).

To deploy Eclipse Che to OpenShift you can use [these templates](https://github.com/eclipse/che/blob/master/deploy/openshift/).

In the output you will get link to the deployed Eclipse Che project. Use it to login to Eclipse Che.
> Notice: for ocp.sh you could use argument `--setup-ocp-oauth`, but in this case you should use "Openshift v3" auth on the login page.

Register new user on the login page. After login you will be redirected to
the Eclipse Che user dashboard.

Create an Eclipse Che 7.x workspace using the default Theia IDE. Then you can [test che-machine-exec with help che-theia-terminal-extension](#test-che-machine-exec-with-help-eclipse-che-theia-terminal) and [test che-machine-exec with help che-theia-task-plugin](#test-che-machine-exec-with-help-che-theia-task-plugin)

## Test on Minishift

First, [build Eclipse Che Assembly](#build-eclipse-che-assembly).

Install minishift with help this instractions:
 - https://docs.okd.io/latest/minishift/getting-started/preparing-to-install.html
 - https://docs.okd.io/latest/minishift/getting-started/setting-up-virtualization-environment.html

Install oc tool: [download oc binary for your platform](https://github.com/openshift/origin/releases), extract and apply this binary path to the system environment variables PATH. After that oc become availiable from terminal:

```bash
$ oc version
oc v3.9.0+191fece
kubernetes v1.9.1+a0ce1bc657
features: Basic-Auth GSSAPI Kerberos SPNEGO
```

Start Minishift:
```bash
$ minishift start --memory=8GB
-- Starting local OpenShift cluster using 'kvm' hypervisor...
...
   OpenShift server started.
   The server is accessible via web console at:
       https://192.168.99.128:8443

   You are logged in as:
       User:     developer
       Password: developer

   To login as administrator:
       oc login -u system:admin
```

From this command output you need:
 - Minishift master url. In this case it's `https://192.168.42.159:8443`, or `CHE_INFRA_KUBERNETES_MASTER__URL`. We can store this variable in the terminal session to use it for next commands:

 ```bash
 export CHE_INFRA_KUBERNETES_MASTER__URL=https://192.168.42.162:8443
 ```
> Note: in case if you delete minishift virtual machine(`minishift delete`) and create it again, this url will be changed.

Register new user on the `CHE_INFRA_KUBERNETES_MASTER__URL` page.

Login to minishift with help oc, use new user login and password for it:

```bash
$ oc login --server=${CHE_INFRA_KUBERNETES_MASTER__URL}
```
This command activates OpenShift context to use minishift instance:

To deploy Eclipse Che you can use [deploy_che.sh script](https://github.com/eclipse/che/blob/master/deploy/openshift/deploy_che.sh).

Move to deploy_che.sh script:
```
cd ~/projects/che/deploy/openshift
```

Run deploy_che.sh script with arguments:

```bash
export CHE_INFRA_KUBERNETES_MASTER__URL=${CHE_INFRA_KUBERNETES_MASTER__URL} && ./deploy_che.sh --no-pull --debug --multiuser
```

Create an Eclipse Che 7.x workspace using the default Theia IDE. Then you can [test che-machine-exec with help che-theia-terminal-extension](#test-che-machine-exec-with-help-eclipse-che-theia-terminal) and [test che-machine-exec with help che-theia-task-plugin](#test-che-machine-exec-with-help-che-theia-task-plugin)

## Test on the Kubernetes (MiniKube)

First, [build Eclipse Che Assembly](#build-eclipse-che-assembly).

Install minikube virtual machine on your computer: https://github.com/kubernetes/minikube/blob/master/README.md

You can deploy Eclipse Che with help helm. So, [install Helm](https://github.com/kubernetes/helm/blob/master/docs/install.md)

Start new minikube:
```bash
minikube start --cpus 2 --memory 8192 --extra-config=apiserver.authorization-mode=RBAC
```

Go to helm/che directory:
```bash
$ cd ~/projects/che/deploy/kubernetes/helm/che
```

- Add cluster-admin role for `kube-system:default` account
```bash
kubectl create clusterrolebinding add-on-cluster-admin --clusterrole=cluster-admin --serviceaccount=kube-system:default
```
- Set your default Kubernetes context:
```bash
kubectl config use-context minikube
```
- Install tiller on your cluster:
  - Create a [tiller serviceAccount]:
    ```bash
    kubectl create serviceaccount tiller --namespace kube-system
    ```
   - Bind it to the almighty cluster-admin role:
      ```bash
      kubectl apply -f ./tiller-rbac.yaml
      ```
  - Install tiller itself:
    ```bash
    helm init --service-account tiller
    ```
- Start NGINX-based ingress controller:
  ```bash
  minikube addons enable ingress
  ```

There are two configurations to deploy Eclipse Che on the Kubernetes:
 - first one: for each new workspace Eclipse Che creates separated namespace:
    ```bash
      helm upgrade --install che --namespace che ./
    ```
 - second one: Eclipse Che creates workspace in the same namespace:
    ```bash
    helm upgrade --install che --namespace=che --set global.cheWorkspacesNamespace=che ./
    ```

> Info: To deploy multi-user Che you can use parameter: `-f ./values/multi-user.yaml`. Also you can set ingress domain with help parameter: `--set global.ingressDomain=<domain>`

> Note: You can track deploy Che with help Minikube dashboard:
  ```bash
  minikube dashboard
  ```

Create an Eclipse Che 7.x workspace using the default Theia IDE. Then you can [test che-machine-exec with help che-theia-terminal-extension](#test-che-machine-exec-with-help-eclipse-che-theia-terminal) and [test che-machine-exec with help che-theia-task-plugin](#test-che-machine-exec-with-help-che-theia-task-plugin)

## Build Eclipse Che Assembly

> Requiements: installed java 8 or higher and maven 3.5 or higher

First, clone Eclipse Che:

```
$ git clone https://github.com/eclipse/che.git ~/projects/che
```

You can save time by simply building the `assembly-main` module, rather than the whole Eclipse Che project.

```
$ cd ~/projects/che/assembly/assembly-main
$ mvn clean install -DskipTests
```

## Test che-machine-exec with help eclipse-che-theia-terminal

Eclipse Che 7.x workspaces using Theia IDE include the che-theia-terminal-extension. You can use this to test che-machine-exec.

In a Che 7 workspace, select: `Terminal` => `Open Terminal in specific container`. Select a container to create new terminal on the bottom panel.

Can also use command palette: `Ctrl + Shift + P` and type `terminal`, then select a container with arrow keys.

## Test che-machine-exec with help che-theia-task-plugin

Eclipse Che 7.x workspaces using Theia IDE include che-theia-task-plugin. You can use this to test che-machine-exec.

Create new Theia task for your project: in the project root create folder `.theia`. Create `tasks.json` file in the folder `.theia` with such content:

```bash
{
    "tasks": [
        {
            "label": "che",
            "type": "che",
            "command": "echo hello"
        }
    ]
}
```
Run this task with help menu tasks: `Terminal` => `Run Task...`
After that Theia should display widget with output content: 'echo hello'

## CI
The following [CentOS CI jobs](https://ci.centos.org/) are associated with the repository:

- [![Master Build Status](https://ci.centos.org/buildStatus/icon?subject=master&job=devtools-che-machine-exec-build-master/)](https://ci.centos.org/job/devtools-che-machine-exec-build-master/) - builds CentOS images on each commit to [`master`](https://github.com/eclipse/che-machine-exec/tree/master) branch and pushes them to [quay.io](https://quay.io/organization/eclipse).
- [![Nightly Build Status](https://ci.centos.org/buildStatus/icon?subject=nightly&job=devtools-che-machine-exec-nightly)](https://ci.centos.org/job/devtools-che-machine-exec-nightly/) - builds CentOS images and pushes them to [quay.io](https://quay.io/organization/eclipse) on a daily basis from the [`master`](https://github.com/eclipse/che-machine-exec/tree/master) branch.
- [![Release Build Status](https://ci.centos.org/buildStatus/icon?subject=release&job=devtools-che-machine-exec-release/)](https://ci.centos.org/job/devtools-che-machine-exec-release/) -  builds images from the [`release`](https://github.com/eclipse/che-machine-exec/tree/release) branch and pushes them to [quay.io](https://quay.io/organization/eclipse).
