#!/usr/bin/env bash
# Creates a NEW development cluster and keeps its credentials in a separate file.
# Never changes the user's current Kubernetes context or an existing cluster.
set -euo pipefail
umask 077

usage() {
  cat <<'EOF'
Usage: scripts/k8s-local.sh [cluster-name [kubeconfig-file]]

Default cluster: aycorn
Default kubeconfig: ${XDG_CONFIG_HOME:-$HOME/.config}/aycorn/kubeconfig-<name>

Requires Docker, kind, kubectl, curl, and sha256sum (or shasum) on PATH.
Creates a new kind cluster, installs the pinned network-policy controller,
and waits for readiness. Refuses an existing cluster or kubeconfig file.
It does not install host tools or change your existing kubeconfig/context.
EOF
}

if [[ ${1:-} == "--help" || ${1:-} == "-h" ]]; then
  usage
  exit 0
fi
if (( $# > 2 )); then
  usage >&2
  exit 2
fi

cluster_name=${1:-aycorn}
if [[ ! $cluster_name =~ ^[a-z0-9]([-a-z0-9]*[a-z0-9])?$ || ${#cluster_name} -gt 50 ]]; then
  printf 'Use a lowercase cluster name of 1–50 letters, digits, or hyphens.\n' >&2
  exit 2
fi
cluster_context="kind-${cluster_name}"
cluster_kubeconfig=${2:-${XDG_CONFIG_HOME:-${HOME}/.config}/aycorn/kubeconfig-${cluster_name}}
script_root=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)

for executable in docker kind kubectl curl; do
  if ! command -v "$executable" >/dev/null 2>&1; then
    printf 'Missing %s on PATH. See Documentation/kubernetes-environments.md.\n' "$executable" >&2
    exit 1
  fi
done
if command -v sha256sum >/dev/null 2>&1; then
  checksum_command=(sha256sum)
elif command -v shasum >/dev/null 2>&1; then
  checksum_command=(shasum -a 256)
else
  printf 'Install sha256sum or shasum before continuing.\n' >&2
  exit 1
fi
if [[ -e $cluster_kubeconfig || -L $cluster_kubeconfig ]]; then
  printf 'Refusing to overwrite kubeconfig: %s\n' "$cluster_kubeconfig" >&2
  exit 1
fi
docker info >/dev/null
existing_clusters=$(KIND_EXPERIMENTAL_PROVIDER=docker kind get clusters)
while IFS= read -r existing_cluster; do
  if [[ $existing_cluster == "$cluster_name" ]]; then
    printf 'Cluster %s already exists. Choose a new name; this script never modifies existing clusters.\n' "$cluster_name" >&2
    exit 1
  fi
done <<< "$existing_clusters"

setup_directory=$(mktemp -d "${TMPDIR:-/tmp}/aycorn-k8s-setup.XXXXXX")
cluster_started=false
setup_complete=false
cleanup() {
  local result=$?
  rm -rf -- "$setup_directory"
  if [[ $cluster_started == true && $setup_complete != true ]]; then
    printf '\nSetup did not finish. Cluster %s and its kubeconfig were retained for diagnosis:\n%s\n' "$cluster_name" "$cluster_kubeconfig" >&2
    printf 'Inspect only this cluster with: kubectl --kubeconfig %q --context %q get pods -A\n' "$cluster_kubeconfig" "$cluster_context" >&2
  fi
  exit "$result"
}
trap cleanup EXIT

# The v1.1.1 upstream manifest intentionally references the v1.1.0 image.
policy_url='https://raw.githubusercontent.com/kubernetes-sigs/kube-network-policies/v1.1.1/install.yaml'
policy_sha256='5491e0364d32e74807bc80abcf4264a2e9b9a98c6d4a96cf55dc7770fae1aa19'
policy_file="${setup_directory}/network-policies.yaml"
curl --fail --location --silent --show-error --retry 2 --connect-timeout 15 --max-time 90 "$policy_url" --output "$policy_file"
actual_checksum=$("${checksum_command[@]}" "$policy_file")
if [[ ${actual_checksum%% *} != "$policy_sha256" ]]; then
  printf 'Network policy manifest checksum mismatch. No cluster was created.\n' >&2
  exit 1
fi

mkdir -p -- "$(dirname -- "$cluster_kubeconfig")"
cluster_kubeconfig="$(cd -- "$(dirname -- "$cluster_kubeconfig")" && pwd)/$(basename -- "$cluster_kubeconfig")"
printf 'Creating new cluster %s. Credentials: %s\n' "$cluster_name" "$cluster_kubeconfig"
cluster_started=true
KIND_EXPERIMENTAL_PROVIDER=docker kind create cluster --name "$cluster_name" --config "${script_root}/ops/kubernetes/kind.yaml" --kubeconfig "$cluster_kubeconfig" --wait 180s
kubectl --kubeconfig "$cluster_kubeconfig" --context "$cluster_context" apply -f "$policy_file"
kubectl --kubeconfig "$cluster_kubeconfig" --context "$cluster_context" -n kube-system rollout status daemonset/kube-network-policies --timeout=180s
kubectl --kubeconfig "$cluster_kubeconfig" --context "$cluster_context" wait --for=condition=Ready nodes --all --timeout=120s
kubectl --kubeconfig "$cluster_kubeconfig" --context "$cluster_context" get storageclasses
setup_complete=true

printf '\nCluster ready. Your previous Kubernetes context is unchanged.\n'
printf 'Launch Aycorn from a shell with:\n  export KUBECONFIG=%q\n' "$cluster_kubeconfig"
printf '\nIn Project Settings → Environments, choose context %s, image delivery Local kind cluster, and cluster name %s.\n' "$cluster_context" "$cluster_name"
printf 'Restart an already-running Aycorn server after changing its PATH or KUBECONFIG.\n'
