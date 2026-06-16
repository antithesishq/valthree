## Kubernetes Manifests for Valthree

A collection of manifests to run valthree in a Kubernetes cluster. 

Antithesis runs a [K3s](https://k3s.io/) cluster in its deterministic environment and uses [kapp](https://carvel.dev/kapp/) to deploy resources in an ordered manner to the cluster.

To build images for valthree to run:

```
podman build -t valthree:latest -f Dockerfile.valthree
```

FYI Podman build stores images without a hostname with a localhost prepended so I have updated the manifests accordingly.

To import the image into k3s:
```
podman save valthree:latest | sudo k3s ctr images import -
```

To view that the image is loaded in k3s:
```
sudo k3s ctr images list
```

To deploy Valthree to a cluster using Kapp:
```
kapp deploy -a valthree -f manifests/
```

To clean up Valthree from the cluster using Kapp:
```
kapp delete -a valthree
```