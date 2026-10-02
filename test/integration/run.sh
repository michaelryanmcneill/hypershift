#!/bin/bash

set -o errexit
set -o nounset
set -o pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "${repo_root}"
export PATH="${repo_root}/bin:${PATH}"

# Integration tests require podman (kind should use the podman provider).
export KIND_EXPERIMENTAL_PROVIDER="${KIND_EXPERIMENTAL_PROVIDER:-podman}"

if ! command -v podman >/dev/null 2>&1 || ! podman info >/dev/null 2>&1; then
  echo "podman is required for integration tests. Install podman and ensure it is running (e.g. 'podman machine start' on macOS)." >&2
  exit 1
fi

workdir="${WORK_DIR:-}"
if [[ -z "${workdir}" ]]; then
  workdir="$( mktemp -d )"
  echo "Placing content in ${workdir}."
else
  mkdir -p "${workdir}"
fi
# Absolute path so printed exports work from any cwd.
workdir="$(cd "${workdir}" && pwd)"

function require_pull_secret() {
  if [[ -z "${PULL_SECRET:-}" ]]; then
    echo "\$PULL_SECRET is required - visit https://console.redhat.com/openshift/create/local to download a secret." >&2
    exit 1
  fi
}

function on_error_exit() {
  local status=$?
  if [[ ${status} -ne 0 ]]; then
    echo "Failed (exit ${status}). To resume with the same workdir:" >&2
    echo "  export WORK_DIR=\"${workdir}\"" >&2
    if [[ -f "${workdir}/kubeconfig" ]]; then
      echo "  export KUBECONFIG=\"${workdir}/kubeconfig\"" >&2
    fi
  fi
}
trap on_error_exit EXIT

# Fail fast before cluster-up/image when a later step needs the pull secret.
for arg in "$@"; do
  case "${arg}" in
    setup|teardown|test)
      require_pull_secret
      break
      ;;
  esac
done

# KIND_NETWORK_POLICY=on|off controls whether the harness applies an allow-all NetworkPolicy
# in each HCP namespace (stock kindnetd enforces NPs and has no disable flag).
# Defaults: off on macOS (kindnet+HO ingress NPs break DNS under podman/Docker Desktop/OrbStack VMs), on on Linux.
function resolve_kind_network_policy() {
  local raw="${KIND_NETWORK_POLICY:-}"
  local normalized
  normalized="$(printf '%s' "${raw}" | tr '[:upper:]' '[:lower:]')"
  if [[ -z "${normalized}" ]]; then
    case "$(uname -s)" in
      Darwin) normalized="off" ;;
      *) normalized="on" ;;
    esac
  fi
  case "${normalized}" in
    on|off)
      KIND_NETWORK_POLICY="${normalized}"
      export KIND_NETWORK_POLICY
      echo "KIND_NETWORK_POLICY=${KIND_NETWORK_POLICY}"
      ;;
    *)
      echo "KIND_NETWORK_POLICY must be 'on' or 'off' (got: ${KIND_NETWORK_POLICY})" >&2
      exit 1
      ;;
  esac
}

kind_cluster_name="integration"
function cluster_up() {
  mkdir -p "${workdir}"
  cat <<EOF >"${workdir}/audit-policy.yaml"
apiVersion: audit.k8s.io/v1
kind: Policy
rules:
- level: Metadata
EOF
  cat <<EOF >"${workdir}/kind-config.yaml"
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
nodes:
- role: control-plane
  kubeadmConfigPatches:
  - |
    kind: ClusterConfiguration
    apiServer:
        # enable auditing flags on the API server
        extraArgs:
          audit-log-path: /var/log/kubernetes/kube-apiserver-audit.log
          audit-policy-file: /etc/kubernetes/policies/audit-policy.yaml
        # mount new files / directories on the control plane
        extraVolumes:
          - name: audit-policies
            hostPath: /etc/kubernetes/policies
            mountPath: /etc/kubernetes/policies
            readOnly: true
            pathType: "DirectoryOrCreate"
          - name: "audit-logs"
            hostPath: "/var/log/kubernetes"
            mountPath: "/var/log/kubernetes"
            readOnly: false
            pathType: DirectoryOrCreate
  # mount the local file on the control plane
  extraMounts:
  - hostPath: ${workdir}/audit-policy.yaml
    containerPath: /etc/kubernetes/policies/audit-policy.yaml
    readOnly: true
EOF
  echo "Setting up kind cluster ${kind_cluster_name}..."
  kind create cluster --name "${kind_cluster_name}" --config "${workdir}/kind-config.yaml"
  kind get kubeconfig --name "${kind_cluster_name}" > "${workdir}/kubeconfig"
  echo "Kind cluster ready. To use it from this shell:"
  echo "  export KUBECONFIG=\"${workdir}/kubeconfig\""
}

function cluster_down() {
  echo "Cleaning up kind cluster ${kind_cluster_name}..."
  kind delete cluster --name "${kind_cluster_name}"
  rm -rf "${workdir}"
}

function audit_log() {
  echo "Fetching audit logs to ${workdir}/kube-apiserver-audit.log"
  podman cp "${kind_cluster_name}-control-plane:/var/log/kubernetes/kube-apiserver-audit.log" "${workdir}/kube-apiserver-audit.log"
}

image_name="quay.io/hypershift/hypershift:integration-image"
function image() {
  echo "Building native binaries..."
  make build
  echo "Cross-compiling Linux binaries for container image..."
  linux_bin="${workdir}/linux-bin"
  mkdir -p "${linux_bin}"
  go_arch="$(go env GOARCH)"
  GOOS=linux GOARCH="${go_arch}" make build \
    'GO_BUILD_RECIPE=CGO_ENABLED=0 $(GO) build $(GO_GCFLAGS) $(GO_LDFLAGS)' \
    "OUT_DIR=${linux_bin}"
  # we don't add the LABEL stanzas to this image, since the HyperShift Operator won't
  # be able to read the image metadata anyway, so we provide it as an annotation on the
  # HostedCluster resources the same way we provide the Control Plane Operator image
  dockerfile="${workdir}/Dockerfile.integration"
  cat <<EOF >"${dockerfile}"
FROM quay.io/fedora/fedora:latest
COPY hypershift-operator control-plane-operator control-plane-pki-operator karpenter-operator hypershift hcp /usr/bin/
ENTRYPOINT ["/usr/bin/hypershift"]
EOF
  podman build -f "${dockerfile}" -t "${image_name}" "${linux_bin}"
  podman save --format docker-archive "${image_name}" -o "${workdir}/integration-image.tar"
  kind load image-archive --name ${kind_cluster_name} "${workdir}/integration-image.tar"
  rm -f "${workdir}/integration-image.tar"
}

function reload() {
  oc --kubeconfig "${workdir}/kubeconfig" get pods --all-namespaces -o json >"${workdir}/pods.json"
  jq --raw-output --arg IMAGE "${image_name}" '.items[] | select(.spec.containers[].image | index($IMAGE)) | "\(.metadata.name) --namespace \(.metadata.namespace)"' <"${workdir}/pods.json" >"${workdir}/args.txt"
  while IFS="" read -r line
  do
    oc --kubeconfig "${workdir}/kubeconfig" delete pod ${line} &
  done < "${workdir}/args.txt"
  for job in $( jobs -p ); do
    wait "${job}"
  done
}

image_labels="$( grep -Eo "LABEL .*" Dockerfile.control-plane | awk '{ print $2}' | paste -sd "," - )"

# Run go test with optional GO_TEST_FLAGS. Avoids nounset/`${VAR:-}` footguns with empty flags.
function run_go_test() {
  local mode="$1"
  shift
  local -a cmd
  cmd=(go test ./test/integration -tags integration -v)
  if [[ -n "${GO_TEST_FLAGS:-}" ]]; then
    # Intentionally unquoted so multiple flags are split.
    # shellcheck disable=SC2206
    cmd+=(${GO_TEST_FLAGS})
  fi
  cmd+=("$@")
  cmd+=(--mode="${mode}")
  "${cmd[@]}"
}

function run_setup() {
  require_pull_secret
  resolve_kind_network_policy
  rm -rf "${workdir}/artifacts"
  mkdir -p "${workdir}/artifacts"
  echo "Running setup..."
  run_go_test setup \
    --timeout 0 \
    --kubeconfig "${workdir}/kubeconfig" \
    --pull-secret "${PULL_SECRET}" \
    --artifact-dir "${workdir}/artifacts" \
    --hypershift-operator-image quay.io/hypershift/hypershift:integration-image \
    --control-plane-operator-image quay.io/hypershift/hypershift:integration-image \
    --control-plane-operator-image-labels "${image_labels}"
  echo "Setup complete. Run './test/integration/run.sh teardown' when finished."
  echo "To use the cluster from this shell:"
  echo "  export KUBECONFIG=\"${workdir}/kubeconfig\""
}

function run_teardown() {
  echo "Running teardown..."
  # Pull secret / images are required by option validation even though teardown only deletes.
  require_pull_secret
  run_go_test teardown \
    --timeout 0 \
    --kubeconfig "${workdir}/kubeconfig" \
    --pull-secret "${PULL_SECRET}" \
    --artifact-dir "${workdir}/artifacts" \
    --hypershift-operator-image quay.io/hypershift/hypershift:integration-image \
    --control-plane-operator-image quay.io/hypershift/hypershift:integration-image \
    --control-plane-operator-image-labels "${image_labels}"
}

function run_test() {
  require_pull_secret
  resolve_kind_network_policy
  echo "Running test..."
  run_go_test test \
    --kubeconfig "${workdir}/kubeconfig" \
    --pull-secret "${PULL_SECRET}" \
    --artifact-dir "${workdir}/artifacts" \
    --hypershift-operator-image quay.io/hypershift/hypershift:integration-image \
    --control-plane-operator-image quay.io/hypershift/hypershift:integration-image \
    --control-plane-operator-image-labels "${image_labels}"
}

for arg in "$@"; do
  case "${arg}" in
    cluster-up)
      cluster_up
      ;;
    cluster-down)
      cluster_down
      ;;
    audit-log)
      audit_log
      ;;
    image)
      image
      ;;
    reload)
      reload
      ;;
    setup)
      run_setup
      ;;
    teardown)
      run_teardown
      ;;
    test)
      run_test
      ;;
    *)
      echo "unknown argument: ${arg}" >&2
      echo "usage: $0 [cluster-up|cluster-down|audit-log|image|reload|setup|teardown|test]..." >&2
      exit 1
      ;;
  esac
done
