# TSP — Troubleshooting Pod

An ephemeral, batteries-included network debugging pod for Kubernetes, plus a
`kubectl` plugin to deploy it into the current namespace with one command.

- **Container image** — a Debian-based image packed with network tooling
  (`tcpdump`, `tshark`, `dig`, `mtr`, `nmap`, `iperf3`, `nft`, `curl`, `openssl`,
  and more). Prints a command cheatsheet when you exec in.
- **`kubectl-tsp` plugin** — deploys the pod, refuses to create a second one in a
  namespace, and lets you pick the pod image version.
- **Release workflow** — a Git tag `vX.Y.Z` builds and publishes both the image
  and the plugin binaries at the same version.

---

## Quick start

```bash
# Deploy a troubleshooting pod into your current namespace
kubectl tsp

# Wait for it, then drop into a shell (you'll get a command cheatsheet)
kubectl exec -it tsp -- bash

# Clean up
kubectl tsp delete
```

---

## The `kubectl-tsp` plugin

`kubectl` discovers any executable named `kubectl-tsp` on your `PATH` and exposes
it as `kubectl tsp`.

### Install

Download the archive for your platform from the
[latest release](https://github.com/cudneys/tsp/releases), then:

```bash
tar -xzf kubectl-tsp_*_linux_amd64.tar.gz
sudo install kubectl-tsp /usr/local/bin/kubectl-tsp
kubectl tsp version
```

Or build it yourself:

```bash
cd plugin
go build -o kubectl-tsp .
sudo install kubectl-tsp /usr/local/bin/kubectl-tsp
```

### Commands

| Command | Description |
|---|---|
| `kubectl tsp` / `kubectl tsp deploy` | Deploy a pod into the current namespace. No-op if one already exists. |
| `kubectl tsp status` | Show the troubleshooting pod in the namespace, if any. |
| `kubectl tsp delete` | Delete the troubleshooting pod. |
| `kubectl tsp version` | Print the plugin version. |

### Flags

| Flag | Default | Description |
|---|---|---|
| `--image-version` | plugin version | Image tag (pod version) to deploy. |
| `--image-repo` | `ghcr.io/cudneys/tsp` | Image repository (without tag). |
| `--pod-name` | `tsp` | Name of the pod to create. |
| `--host-network` | `false` | Run in the **node's** network namespace to debug node-level networking (sets `hostNetwork` + `ClusterFirstWithHostNet` DNS). |
| `--security-profile` | `default` | Security posture: `default`, `baseline`, or `restricted` (see [PodSecurity](#podsecurity--capabilities)). |
| `--dry-run` | `false` | Print the pod manifest that would be created and exit (no cluster changes). |
| `--namespace`, `--context`, `--kubeconfig`, … | — | Standard `kubectl` config flags. |

The pod version **defaults to the plugin's own version**, so a plugin built at
`1.2.3` deploys `ghcr.io/cudneys/tsp:1.2.3` unless you pass `--image-version`.

### Examples

```bash
# Pin a specific pod image version
kubectl tsp deploy --image-version 1.1.0

# Debug the node's network stack rather than the pod's
kubectl tsp deploy --host-network

# Target a specific namespace
kubectl tsp deploy -n kube-system
```

### Namespace safety

Before deploying, the plugin lists pods labeled `app=tsp` in the current
namespace. If one already exists it prints how to connect to it (or remove it)
and exits without creating a duplicate.

---

## The pod

When you exec into the pod you get a login banner: a categorized table of every
installed tool, and a footer showing the pod name, namespace, and node — the
last two are injected via the Kubernetes **Downward API**:

| Env var | Downward API field |
|---|---|
| `POD_NAME` | `metadata.name` |
| `POD_NAMESPACE` | `metadata.namespace` |
| `NODE_NAME` | `spec.nodeName` |

### PodSecurity & capabilities

By default the pod adds `NET_RAW` + `NET_ADMIN` so raw-socket tools and
route/firewall manipulation work. But clusters that enforce the
[PodSecurity](https://kubernetes.io/docs/concepts/security/pod-security-standards/)
`baseline` or `restricted` standard (e.g. Talos out of the box) reject added
capabilities:

```
pods "tsp" is forbidden: violates PodSecurity "baseline:latest":
non-default capabilities (must not include "NET_ADMIN", "NET_RAW" ...)
```

Use `--security-profile` to pick a compliant posture:

| Profile | securityContext | Works | Doesn't work |
|---|---|---|---|
| `default` | adds `NET_RAW`, `NET_ADMIN` | everything | needs a privileged/unrestricted namespace |
| `baseline` | no added capabilities | `tcpdump`, `tshark`, `ping`, `nmap`, and all connect tools | `nft`/route edits (`NET_ADMIN`) |
| `restricted` | drop `ALL`, non-root, seccomp, no priv-esc | connect-only: `curl`, `dig`, `nc`, `nmap -sT`, `iperf3`, `ss`, `jq` | raw sockets: `tcpdump`, `ping`, raw `nmap` |

The key insight: the container runtime's **default capability set already
includes `NET_RAW`**, so `baseline` keeps packet capture and ping working — it
only loses `NET_ADMIN`, which you need solely to *modify* routes/nftables.

```bash
# Talos / baseline-enforced clusters
kubectl tsp deploy --security-profile baseline

# Preview the exact manifest for any profile without deploying
kubectl tsp deploy --security-profile restricted --dry-run
```

### Manifests

The plugin embeds its manifest, but standalone copies are provided for plain
`kubectl apply`:

- `tsp-pod.yaml` — one-shot debug Pod
- `tsp-deployment.yaml` — long-lived Deployment

```bash
kubectl apply -f tsp-pod.yaml
```

> Update the `image:` field to point at your registry (e.g.
> `ghcr.io/cudneys/tsp:1.2.3`) before applying.

---

## Releasing

Releases are cut by pushing a semver tag. The
[`release` workflow](.github/workflows/release.yml) then:

1. Builds the container image (`linux/amd64` + `linux/arm64`) and pushes it to
   `ghcr.io/cudneys/tsp` tagged with **only** the exact version (`1.2.3`) —
   never `1`, `1.2`, or `latest`.
2. Builds the `kubectl-tsp` plugin at the **same version** for linux, macOS, and
   Windows, and attaches the archives + checksums to a GitHub release.

```bash
git tag v1.2.3
git push origin v1.2.3
```

---

## Repository layout

```
.
├── Dockerfile               # troubleshooting-pod image
├── motd.sh                  # login banner baked into the image
├── tsp-pod.yaml             # standalone Pod manifest
├── tsp-deployment.yaml      # standalone Deployment manifest
├── plugin/                  # kubectl-tsp Go plugin
│   ├── main.go
│   └── manifests/tsp-pod.yaml   # manifest embedded into the binary
└── .github/workflows/release.yml
```
