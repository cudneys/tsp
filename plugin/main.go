// Command kubectl-tsp is a kubectl plugin that deploys an ephemeral network
// troubleshooting pod ("tsp") into the current namespace.
//
// Invoked as `kubectl tsp [deploy|delete|status|version] [flags]`.
package main

import (
	"context"
	_ "embed"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/pflag"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/yaml"
)

//go:embed manifests/tsp-pod.yaml
var podManifest []byte

// These are overridden at build time via -ldflags "-X main.version=... -X main.defaultImageRepo=...".
var (
	// version is the plugin's own version and the default pod image tag.
	version = "dev"
	// defaultImageRepo is the container image repository (without tag) the pod runs.
	defaultImageRepo = "ghcr.io/cudneys/tsp"
)

// selector used to detect an existing troubleshooting pod in the namespace.
const managedSelector = "app=tsp"

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	// Split off an optional subcommand (first non-flag token). Default: deploy.
	subcommand := "deploy"
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		subcommand = args[0]
		args = args[1:]
	}

	flags := pflag.NewFlagSet("kubectl-tsp", pflag.ContinueOnError)
	flags.Usage = func() { printUsage(flags) }

	// Standard kube flags: --namespace, --context, --kubeconfig, etc.
	configFlags := genericclioptions.NewConfigFlags(true)
	configFlags.AddFlags(flags)

	imageVersion := flags.String("image-version", version,
		"Version (image tag) of the troubleshooting pod to deploy. Defaults to the plugin version.")
	imageRepo := flags.String("image-repo", defaultImageRepo,
		"Container image repository (without tag) for the troubleshooting pod.")
	podName := flags.String("pod-name", "tsp", "Name of the pod to create.")
	hostNetwork := flags.Bool("host-network", false,
		"Run the pod in the host's network namespace (sets hostNetwork + ClusterFirstWithHostNet DNS) to debug node-level networking.")
	securityProfile := flags.String("security-profile", "default",
		"Security profile: 'default' (adds NET_RAW+NET_ADMIN), 'baseline' (no added caps; PodSecurity baseline-compatible), or 'restricted' (fully locked down, connect-only tools).")
	dryRun := flags.Bool("dry-run", false, "Print the pod manifest that would be created and exit (no cluster changes).")
	help := flags.BoolP("help", "h", false, "Show help.")

	if err := flags.Parse(args); err != nil {
		return err
	}
	if *help || subcommand == "help" {
		printUsage(flags)
		return nil
	}

	switch subcommand {
	case "version":
		fmt.Printf("kubectl-tsp %s\n", version)
		return nil
	case "deploy", "delete", "status":
		// handled below
	default:
		return fmt.Errorf("unknown command %q (want: deploy, delete, status, version)", subcommand)
	}

	// Resolve the current namespace (honours --namespace and the kubeconfig context).
	namespace, _, err := configFlags.ToRawKubeConfigLoader().Namespace()
	if err != nil {
		return fmt.Errorf("resolving namespace: %w", err)
	}

	restConfig, err := configFlags.ToRESTConfig()
	if err != nil {
		return fmt.Errorf("loading kube config: %w", err)
	}
	clientset, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return fmt.Errorf("building kubernetes client: %w", err)
	}
	ctx := context.Background()

	switch subcommand {
	case "status":
		return statusPod(ctx, clientset, namespace)
	case "delete":
		return deletePod(ctx, clientset, namespace, *podName)
	default: // deploy
		return deployPod(ctx, clientset, namespace, deployOptions{
			podName:         *podName,
			imageRepo:       *imageRepo,
			imageTag:        *imageVersion,
			hostNetwork:     *hostNetwork,
			securityProfile: *securityProfile,
			dryRun:          *dryRun,
		})
	}
}

// deployOptions captures everything that shapes the pod to be created.
type deployOptions struct {
	podName         string
	imageRepo       string
	imageTag        string
	hostNetwork     bool
	securityProfile string
	dryRun          bool
}

// existingTSP returns the first non-terminating pod matching the managed
// selector in the namespace, or nil if none exist.
func existingTSP(ctx context.Context, c kubernetes.Interface, namespace string) (*corev1.Pod, error) {
	list, err := c.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{LabelSelector: managedSelector})
	if err != nil {
		return nil, fmt.Errorf("listing existing pods: %w", err)
	}
	for i := range list.Items {
		if list.Items[i].DeletionTimestamp == nil {
			return &list.Items[i], nil
		}
	}
	return nil, nil
}

func deployPod(ctx context.Context, c kubernetes.Interface, namespace string, opts deployOptions) error {
	// Build the pod spec first so --dry-run works without touching the cluster.
	pod := &corev1.Pod{}
	if err := yaml.Unmarshal(podManifest, pod); err != nil {
		return fmt.Errorf("decoding embedded manifest: %w", err)
	}
	pod.Name = opts.podName
	pod.Namespace = namespace
	if opts.hostNetwork {
		// Share the node's network namespace and resolve cluster DNS through it.
		pod.Spec.HostNetwork = true
		pod.Spec.DNSPolicy = corev1.DNSClusterFirstWithHostNet
	}
	image := fmt.Sprintf("%s:%s", opts.imageRepo, opts.imageTag)
	for i := range pod.Spec.Containers {
		if pod.Spec.Containers[i].Name == "tsp" {
			pod.Spec.Containers[i].Image = image
		}
	}
	if err := applySecurityProfile(pod, opts.securityProfile); err != nil {
		return err
	}

	if opts.dryRun {
		out, err := yaml.Marshal(pod)
		if err != nil {
			return fmt.Errorf("marshalling pod: %w", err)
		}
		fmt.Print(string(out))
		return nil
	}

	// Pre-check: never deploy a second troubleshooting pod into the namespace.
	if existing, err := existingTSP(ctx, c, namespace); err != nil {
		return err
	} else if existing != nil {
		fmt.Printf("A troubleshooting pod already exists in namespace %q: %s (%s)\n",
			namespace, existing.Name, existing.Status.Phase)
		fmt.Printf("Connect with:\n  kubectl exec -it -n %s %s -- bash\n", namespace, existing.Name)
		fmt.Printf("Or remove it with:\n  kubectl tsp delete -n %s\n", namespace)
		return nil
	}

	created, err := c.CoreV1().Pods(namespace).Create(ctx, pod, metav1.CreateOptions{})
	if err != nil {
		if apierrors.IsAlreadyExists(err) {
			return fmt.Errorf("pod %q already exists in namespace %q", opts.podName, namespace)
		}
		return fmt.Errorf("creating pod: %w", err)
	}

	netMode := "pod network"
	if opts.hostNetwork {
		netMode = "host network"
	}
	fmt.Printf("Deployed troubleshooting pod %q in namespace %q (image %s, %s, %s profile)\n",
		created.Name, namespace, image, netMode, opts.securityProfile)
	fmt.Printf("Wait for it to be ready, then connect:\n")
	fmt.Printf("  kubectl wait -n %s --for=condition=Ready pod/%s\n", namespace, created.Name)
	fmt.Printf("  kubectl exec -it -n %s %s -- bash\n", namespace, created.Name)
	return nil
}

// applySecurityProfile rewrites each container's securityContext according to
// the requested PodSecurity posture.
//
//   - default:    keep the manifest's NET_RAW + NET_ADMIN (needs a privileged
//     or unrestricted namespace).
//   - baseline:   add no capabilities. The runtime's default set still includes
//     NET_RAW, so tcpdump/tshark/ping/nmap keep working; only NET_ADMIN
//     (route/nftables edits) is lost. Satisfies the PodSecurity "baseline" level.
//   - restricted: drop ALL capabilities, run as non-root with a seccomp profile
//     and no privilege escalation. Satisfies "restricted"; only connect-based
//     tools (curl, dig, nc, nmap -sT, iperf3, ss, jq) work — no raw sockets.
func applySecurityProfile(pod *corev1.Pod, profile string) error {
	switch profile {
	case "default":
		return nil
	case "baseline", "restricted":
		// handled below
	default:
		return fmt.Errorf("unknown security profile %q (want: default, baseline, restricted)", profile)
	}

	for i := range pod.Spec.Containers {
		sc := pod.Spec.Containers[i].SecurityContext
		if sc == nil {
			sc = &corev1.SecurityContext{}
		}
		switch profile {
		case "baseline":
			// Drop the explicit add list; rely on the default capability set.
			sc.Capabilities = nil
		case "restricted":
			sc.Capabilities = &corev1.Capabilities{Drop: []corev1.Capability{"ALL"}}
			sc.AllowPrivilegeEscalation = boolPtr(false)
			sc.RunAsNonRoot = boolPtr(true)
			sc.RunAsUser = int64Ptr(65532)
			sc.SeccompProfile = &corev1.SeccompProfile{Type: corev1.SeccompProfileTypeRuntimeDefault}
		}
		pod.Spec.Containers[i].SecurityContext = sc
	}
	return nil
}

func boolPtr(b bool) *bool    { return &b }
func int64Ptr(i int64) *int64 { return &i }

func deletePod(ctx context.Context, c kubernetes.Interface, namespace, podName string) error {
	err := c.CoreV1().Pods(namespace).Delete(ctx, podName, metav1.DeleteOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			fmt.Printf("No troubleshooting pod %q in namespace %q\n", podName, namespace)
			return nil
		}
		return fmt.Errorf("deleting pod: %w", err)
	}
	fmt.Printf("Deleting troubleshooting pod %q in namespace %q\n", podName, namespace)
	return nil
}

func statusPod(ctx context.Context, c kubernetes.Interface, namespace string) error {
	existing, err := existingTSP(ctx, c, namespace)
	if err != nil {
		return err
	}
	if existing == nil {
		fmt.Printf("No troubleshooting pod in namespace %q\n", namespace)
		return nil
	}
	image := ""
	if len(existing.Spec.Containers) > 0 {
		image = existing.Spec.Containers[0].Image
	}
	fmt.Printf("%s\t%s\t%s\n", existing.Name, existing.Status.Phase, image)
	return nil
}

func printUsage(flags *pflag.FlagSet) {
	fmt.Fprintf(os.Stderr, `kubectl-tsp %s — deploy an ephemeral network troubleshooting pod.

Usage:
  kubectl tsp [command] [flags]

Commands:
  deploy    Deploy a troubleshooting pod (default). No-op if one already exists.
  delete    Delete the troubleshooting pod in the current namespace.
  status    Show the troubleshooting pod in the current namespace, if any.
  version   Print the plugin version.

Flags:
%s`, version, flags.FlagUsages())
}
