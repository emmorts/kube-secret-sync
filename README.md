# kube-secret-sync

**kube-secret-sync** is a Kubernetes controller that automatically syncs secrets across namespaces based on provided configuration. It simplifies the management of secrets in multi-tenant environments by ensuring that specific secrets are available in the required namespaces.

## Features

- Sync secrets from a source namespace to target namespaces based on pod image matching
- Configurable via environment variables
- Automatic secret cloning when new pods are created or updated
- Efficient processing using informers and work queues
- Retries on conflicts to handle concurrent updates

## Table of Contents

- [Configuration](#configuration)
- [Usage](#usage)

## Configuration

The controller is configured using the `SYNC_CONFIGS` environment variable. It allows you to specify multiple secret sync configurations separated by a semicolon (`;`). Each configuration consists of three fields separated by a comma (`,`):

1. Secret name: The name of the secret to be synced.
2. Source namespace: The namespace where the secret originates from.
3. Target image: A string that should be contained in the pod's image name for the secret to be synced to the pod's namespace.

Example configuration:

```
SYNC_CONFIGS="github-credentials,ns1,ghcr.io;gitlab-credentials,ns2,st0.foolab.com"
```

In this example, the controller will:
- Sync the secret `github-credentials` from namespace `ns1` to all namespaces that contain pods with an image name containing the string `github.io`.
- Sync the secret `gitlab-credentials` from namespace `ns2` to all namespaces that contain pods with an image name containing the string `st0.foolab.com`.

## Usage

TBD