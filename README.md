# TSP — Troubleshooting Pod

An ephemeral, batteries-included network debugging pod for Kubernetes, plus a
`kubectl` plugin to deploy it into the current namespace with one command.

- **Container image** — a Debian-based image packed with network tooling
  (`tcpdump`, `tshark`, `dig`, `mtr`, `nmap`, `iperf3`, `nft`, `curl`, `openssl`,
  and more). Prints a command cheatsheet when you exec in.
- **`kubectl-tsp` plugin** — deploys the pod, refuses to create a second one in a
  namespace, and lets you pick the pod image version.

---

### Install

**Homebrew (recommended, macOS/Linux):**

```bash
brew install cudneys/tap/kubectl-tsp
kubectl tsp version
```

This is the smoothest path on macOS: Homebrew de-quarantines the binary, so
there is **no Gatekeeper prompt** (the "unverified developer" dialog you get
from a raw browser download).

**Manual download:**

```bash
# Grab the archive for your platform from the latest release:
#   https://github.com/cudneys/tsp/releases
tar -xzf kubectl-tsp_*_darwin_arm64.tar.gz

# macOS only: clear the download quarantine so Gatekeeper doesn't block it.
xattr -d com.apple.quarantine kubectl-tsp 2>/dev/null || true

sudo install kubectl-tsp /usr/local/bin/kubectl-tsp
kubectl tsp version
```

**Build it yourself:**

```bash
cd plugin
go build -o kubectl-tsp .
sudo install kubectl-tsp /usr/local/bin/kubectl-tsp
```

### Commands

| Command | Description |
|---|---|
| `kubectl tsp` / `kubectl tsp deploy` | Deploy a pod (or attach to an existing one), wait for readiness, and exec into a shell. |
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
| `--no-exec` | `false` | Deploy only — don't wait for readiness or exec into the pod. |
| `--timeout` | `2m` | How long to wait for the pod to become ready before exec'ing in. |
| `--rm` | `false` | Delete the pod when the exec session ends (like `kubectl run --rm -it`). |
| `--ttl` | `0` (off) | Max pod lifetime (e.g. `1h`); Kubernetes terminates it after this via `activeDeadlineSeconds`. A backstop for abandoned sessions. |
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

# Ephemeral: delete the pod as soon as you exit the shell
kubectl tsp --rm

# Self-cleaning with a safety cap: removed on exit, or killed by Kubernetes
# after 1h if the session is abandoned
kubectl tsp --rm --ttl 1h
```

### Namespace safety

Before deploying, the plugin lists pods labeled `app=tsp` in the current
namespace. If one already exists it prints how to connect to it (or remove it)
and exits without creating a duplicate.

### Cleanup

A standalone Pod is **never auto-deleted** by Kubernetes: `restartPolicy: Never`
(which the pod uses) only stops the *container* from restarting — when it exits
or the node reboots, the Pod object lingers in `Completed`/`Failed`. There is no
built-in TTL for bare Pods. So cleanup is explicit:

| Mechanism | What it does | Removes the Pod object? |
|---|---|---|
| `kubectl tsp delete` | Deletes the troubleshooting pod in the namespace. | ✅ |
| `--rm` | Deletes the pod when your exec session ends (client-side). | ✅ |
| `--ttl <dur>` | Sets `activeDeadlineSeconds`; **Kubernetes** stops the pod after the duration. | ❌ stops it (→ `DeadlineExceeded`), object remains until GC |

The two flags compose: `--rm` handles the normal "clean up when I'm done" exit,
while `--ttl` is a server-side backstop that fires even if the session is
abandoned (laptop sleeps, plugin killed, network drops).

```bash
# Deleted the moment you exit the shell
kubectl tsp --rm

# Deleted on exit, or force-stopped by Kubernetes after 1h if abandoned
kubectl tsp --rm --ttl 1h
```

> `--ttl` alone caps the pod's *runtime* but leaves a stopped Pod object behind;
> pair it with `--rm` (or run `kubectl tsp delete`) to remove the object. For a
> guaranteed server-side deletion you'd model the pod as a Job with
> `ttlSecondsAfterFinished` — overkill for an interactive debug pod.

---

## The pod

The default interactive shell is **zsh** (bash is also available). When you exec
into the pod you get a login banner: a categorized table of every installed
tool, and a footer showing the pod name, namespace, and node — the last two are
injected via the Kubernetes **Downward API**:

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

