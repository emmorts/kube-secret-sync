# kube-secret-sync Helm Chart

This Helm chart deploys the `kube-secret-sync` controller in a Kubernetes cluster.

## Installing the Chart

To install the chart with the release name `my-release`:

```
helm install my-release kube-secret-sync/kube-secret-sync
```

## Uninstalling the Chart

To uninstall the `my-release` deployment:

```
helm uninstall my-release
```

## Configuration

The following table lists the configurable parameters of the `kube-secret-sync` chart and their default values.

| Parameter                | Description                                                                                    | Default                                        |
|--------------------------|------------------------------------------------------------------------------------------------|------------------------------------------------|
| `replicaCount`           | Number of replicas of the `kube-secret-sync` controller                                        | `1`                                            |
| `image.repository`       | `kube-secret-sync` image repository                                                            | `ghcr.io/emmorts/kube-secret-sync`             |
| `image.pullPolicy`       | Image pull policy                                                                              | `Always`                                       |
| `image.tag`              | `kube-secret-sync` image tag                                                                   | `latest`                                       |
| `imagePullSecrets`       | Image pull secrets                                                                             | `[]`                                           |
| `nameOverride`           | String to partially override the fullname template                                             | `""`                                           |
| `fullnameOverride`       | String to fully override the fullname template                                                 | `""`                                           |
| `serviceAccount.create`  | Specifies whether a service account should be created                                          | `true`                                         |
| `serviceAccount.annotations` | Annotations to add to the service account                                                  | `{}`                                           |
| `serviceAccount.name`    | The name of the service account to use                                                         | `""`                                           |
| `configuration.SYNC_CONFIGS` | Secret sync configurations in the format "secretName,sourceNamespace,targetImage;..."       | `""`                                           |
| `podAnnotations`         | Annotations to add to the `kube-secret-sync` pod                                               | `{}`                                           |
| `podSecurityContext`     | Security context for the `kube-secret-sync` pod                                                | `{}`                                           |
| `securityContext`        | Security context for the `kube-secret-sync` container                                          | `{}`                                           |
| `resources`              | CPU/memory resource requests/limits                                                            | `{}`                                           |
| `autoscaling`            | Autoscaling configuration                                                                      | `{}`                                           |
| `nodeSelector`           | Node labels for pod assignment                                                                 | `{}`                                           |
| `tolerations`            | Tolerations for pod assignment                                                                 | `[]`                                           |
| `affinity`               | Affinity for pod assignment                                                                    | `{}`                                           |

Specify each parameter using the `--set key=value[,key=value]` argument to `helm install`. For example:

```
helm install my-release kube-secret-sync/kube-secret-sync \
  --set configuration.SYNC_CONFIGS="github-credentials,ns1,ghcr.io;gitlab-credentials,ns2,st0.foolab.com"
```

Alternatively, a YAML file that specifies the values for the parameters can be provided while installing the chart.