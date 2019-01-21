# CHE machine exec

Go-lang server side to creation machine-execs for Eclipse CHE workspaces.
Uses to spawn terminal or command processes.

CHE machine exec uses json-rpc protocol to communication with client.

# Build docker image

Build docker image with che-machine-exec manually:

```bash
docker build --no-cache -t eclipse/che-machine-exec .
```

# Run docker container

Run docker container with che-machine-exec manually:

```bash
docker run --rm -p 4444:4444 -v /var/run/docker.sock:/var/run/docker.sock eclipse/che-machine-exec
```

# How to use machine-exec image with Eclipse CHE workspace on the docker infrastructure

To configure Eclipse CHE on the docker infrastructure we are using che.env configuration file.
che.env file located in the CHE `data` folder. Edit and save che.env file: apply docker.sock path (by default it's `/var/run/docker.sock`) to the workspace volume property `CHE_WORKSPACE_VOLUME`:

Example:
 ```bash
CHE_WORKSPACE_VOLUME=/var/run/docker.sock:/var/run/docker.sock;
```
 > Notice: all configuration changes become avaliable after restart Eclipse CHE.

## Test che-machine-exec with help UI on the docker infrastructure

Run Eclipse CHE. You can create new Eclipse CHE workspace with integrated Theia IDE from stack 'Theia IDE on docker'. Then You can [test che-machine-exec with help eclipse-che-theia-terminal](#test-che-machine-exec-with-help-eclipse-che-theia-terminal) and [test che-machine-exec with help che-theia-task-plugin](#test-che-machine-exec-with-help-che-theia-task-plugin)

# Test che-machine-exec on the local Openshift

To test che-machine-exec You need deploy Eclipse CHE to the openshift locally. [Prepare Eclipse CHE to deploy](#prepare-eclipse-che-to-deploy)

To deploy Eclipse CHE to the local running openshift You can use [ocp.sh sript](https://github.com/eclipse/che/blob/master/deploy/openshift/ocp.sh).

Move to the ocp.sh script:

```bash
cd ~/projects/che/deploy/openshift/
```

 Run ocp.sh with arguments:

```bash
./ocp.sh --run-ocp --deploy-che --no-pull --debug --deploy-che-plugin-registry --multiuser
```
In the output You will get link to the deployed Eclipse CHE project. Use it to login to Eclipse CHE.
> Notice: for ocp.sh You could use argument `--setup-ocp-oauth`, but in this case
You should use "Openshift v3" auth on the login page.

Register new user on the login page. After login You will be redirected to
the Eclipse CHE user dashboard.

Create new workspace from openshift stack 'Java Theia on OpenShift' or 'CHE 7' stack. Run workspace. When workspace will be running You will see Theia IDE. Then You can [test che-machine-exec with help che-theia-terminal-extension](#test-che-machine-exec-with-help-eclipse-che-theia-terminal) and [test che-machine-exec with help che-theia-task-plugin](#test-che-machine-exec-with-help-che-theia-task-plugin)

# Test on the Minishift
To test che-machine-exec You need deploy Eclipse CHE to the Minishift. [Prepare Eclipse CHE to deploy](#prepare-eclipse-che-to-deploy).

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

From this command output You need:
 - Minishift master url. In this case it's `https://192.168.42.159:8443`. Let's call it 'CHE_INFRA_KUBERNETES_MASTER__URL'. We can store this variable in the terminal session to use it for next commands:

 ```bash
 export CHE_INFRA_KUBERNETES_MASTER__URL=https://192.168.42.162:8443
 ```
> Note: in case if You delete minishift virtual machine(`minishift delete`) and create it again, this url will be changed.

Register new user on the CHE_INFRA_KUBERNETES_MASTER__URL page.

Login to minishift with help oc, use new user login and password for it:

```bash
$ oc login --server=${CHE_INFRA_KUBERNETES_MASTER__URL}
```
This command activates openshift context to use minishift instance:

To deploy Eclipse CHE You can use [deploy_che.sh script](https://github.com/eclipse/che/blob/master/deploy/openshift/deploy_che.sh).

Move to deploy_che.sh script:
```
cd ~/projects/che/deploy/openshift
```

Run deploy_che.sh script with arguments:

```bash
export CHE_INFRA_KUBERNETES_MASTER__URL=${CHE_INFRA_KUBERNETES_MASTER__URL} && ./deploy_che.sh --no-pull --debug --multiuser
```

Create new workspace from openshift stack 'Java Theia on OpenShift' or
'CHE 7' stack. Run workspace. When workspace will be running You will see Theia IDE. Then You can [test che-machine-exec with help che-theia-terminal-extension](#test-che-machine-exec-with-help-eclipse-che-theia-terminal) and [test che-machine-exec with help che-theia-task-plugin](#test-che-machine-exec-with-help-che-theia-task-plugin)

# Test on the Kubernetes (MiniKube)

To test che-machine-exec You need deploy Eclipse CHE to the Minikube cluster. [Prepare Eclipse CHE to deploy](#prepare-eclipse-che-to-deploy).

Install minikube virtual machine on your computer: https://github.com/kubernetes/minikube/blob/master/README.md

You can deploy Eclipse CHE with help helm. So, [install Helm](https://github.com/kubernetes/helm/blob/master/docs/install.md)

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

There are two configurations to deploy Eclipse CHE on the Kubernetes:
 - first one: for each new workspace Eclipse CHE creates separated namespace:
    ```bash
      helm upgrade --install che --namespace che ./
    ```
 - second one: Eclipse CHE creates workspace in the same namespace:
    ```bash
    helm upgrade --install che --namespace=che --set global.cheWorkspacesNamespace=che ./
    ```

> Info: To delploy multi-user CHE you can use parameter: `-f ./values/multi-user.yaml`. Also You can set ingress domain with help parameter: `--set global.ingressDomain=<domain>`

> Notice: You can track deploy CHE with help Minikube dashboard:
  ```bash
  minikube dashboard
  ```

Create new workspace from stack 'CHE 7'. Run workspace. When workspace will be running You will see Theia IDE. Then You can [test che-machine-exec with help che-theia-terminal-extension](#test-che-machine-exec-with-help-eclipse-che-theia-terminal) and [test che-machine-exec with help che-theia-task-plugin](#test-che-machine-exec-with-help-che-theia-task-plugin)

# Prepare Eclipse CHE to deploy

> Requiements: installed java 8 or higher and maven 3.3.0 or higher

First of all clone Eclipse CHE:

```
$ git clone https://github.com/eclipse/che.git ~/projects/che
```

For test purpose it's not nessure build all Eclipse CHE, build 'assembly-main' maven module is pretty enough:

```
$ cd ~/projects/che/assembly/assembly-main
$ mvn clean install -DskipTests
```

# Test che-machine-exec with help eclipse-che-theia-terminal
Eclipse CHE workspace created from Theia stack contains included che-theia-terminal-extension. With help this extension You can test che-machine-exec.

Create new terminal with help main menu of the Theia: `Terminal` => `Open Terminal in specific container`. After that IDE will propose You select machine to creation terminal. Select one of the machines by click. After that Theia should create new terminal on the bottom panel.
Or You could use command palette: `Ctrl + Shift + P` and type `terminal`. Than You could select with help keys `Arrow Up` or `Arrow Down` command for terminal creation and launch it by click on `Enter`.

# Test che-machine-exec with help che-theia-task-plugin

Eclipse CHE workspace created from Theia stack contains included che-theia-task-plugin. With help this plugin You can test che-machine-exec.
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
