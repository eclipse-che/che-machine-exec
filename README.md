# CHE macine exec

Go-lang server side to creation machine-execs for Eclipse CHE workspaces.
Uses to spawn terminal or command processes.

CHE machine exec uses json-rpc protocol to communication with client.

# How to use machine-exec image with Eclipse CHE workspace on the docker infrastructure:
Apply docker.sock path (by default it's `/var/run/docker.sock`) to the workspace volume property `CHE_WORKSPACE_VOLUME` in the che.env file:
Example:
 ```
CHE_WORKSPACE_VOLUME=/var/run/docker.sock
```
che.env file located in the CHE `data` folder. che.env file contains configuration properties for Eclipse CHE. All changes of the file become avaliable after restart Eclipse CHE.
 
# Build docker image

Build docker image with che-machine-exec manually:

```
docker build -t eclipse/che-machine-exec .
```

# Run docker container

Run docker container with che-machine-exec manually:

```
 docker run --rm -p 4444:4444 -v /var/run/docker.sock:/var/run/docker.sock eclipse/che-machine-exec
````
