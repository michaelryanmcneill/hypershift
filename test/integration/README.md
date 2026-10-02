# `kind`-Based Integration Testing

This test framework has a two-fold goal: provide a short iterating cycle for work on HyperShift operators locally
as well as a quick validation harness for features that do not require cloud-provider-specific functionality.

## Local Operation

The `run.sh` script allows for local iteration on HyperShift - use a shell to set up the environment and keep it
running, while using other shells to interact with the environment or even iterate on tests.

### Prerequisites

Make sure you have `kind` and **podman** installed and running (`podman machine start` on macOS).
The script sets `KIND_EXPERIMENTAL_PROVIDER=podman` by default so kind uses podman.
Keep an eye out for `too many open files` errors when launching `HostedCluster`s and apply the [remedy](https://kind.sigs.k8s.io/docs/user/known-issues/#pod-errors-due-to-too-many-open-files).

The operator image is built by cross-compiling Linux binaries (`GOOS=linux`, host `GOARCH`) with `CGO_ENABLED=0`
and copying them into `quay.io/fedora/fedora:latest`. This works on both Linux and macOS (including Apple Silicon).

Visit the [web console](https://console.redhat.com/openshift/create/local) to create a local pull secret -
this is required to interrogate OCP release bundles.

Set up the following environment variables (from the repository root):

```shell
export PATH="${PWD}/bin:${PATH}"  # run.sh also prepends repo bin/ automatically
export WORK_DIR=/tmp/integration # this directory is persistent between runs, cleared only as necessary
export PULL_SECRET="REPLACE-ME"  # point this environment variable at the pull secret you generated
```

#### `KIND_NETWORK_POLICY`

HyperShift installs ingress NetworkPolicies in HostedControlPlane namespaces, and kindnet enforces them.
That often breaks DNS from HCP pods under kind. When NetworkPolicy enforcement is treated as `off`,
the harness applies an allow-all NetworkPolicy in each HCP namespace (stock kindnetd has no disable flag).

| Value | Behavior |
|-------|----------|
| unset on macOS | `off` |
| unset on Linux | `on` |
| `off` | Apply an allow-all NetworkPolicy in each HCP namespace |
| `on` | Leave NetworkPolicies alone (use this when intentionally testing NetworkPolicies) |

```shell
export KIND_NETWORK_POLICY=on   # keep real NetworkPolicy enforcement
export KIND_NETWORK_POLICY=off  # apply allow-all NetworkPolicy workaround
```

### Setup

Run the following to create the kind cluster, build/load the HyperShift image, and install the operator
plus HostedClusters for the selected tests. **Setup exits when complete** (it does not hold the terminal).
Keep `${WORK_DIR}/artifacts` around until you tear down — teardown reads the rendered YAML from there.

# TODO: add some mechanism to choose which tests the setup runs for
```shell
./test/integration/run.sh \
  cluster-up \ # start the kind cluster
  image \      # build the container image for HyperShift, load it into the cluster
  setup        # install HyperShift operator and HostedClusters, then exit
```

After `cluster-up`, the script prints an absolute `KUBECONFIG` export you can copy into other shells.

### Tests

Run the following for quick iteration - the test process will expect that setup is complete. Use `${GO_TEST_FLAGS}` to
specify what subset of the tests to run.

```shell
./test/integration/run.sh \
  test # run tests
```

#### Running A Subset Of Tests

When running the `setup` or `test` targets in `run.sh`, provide `$GO_TEST_FLAGS='-run selector'` to only set up and run
some subset of tests.

### Teardown

Remove HostedClusters, the HyperShift Operator, CRDs, and static assets installed by `setup`. This does **not**
delete the kind cluster — use `cluster-down` for that.

```shell
./test/integration/run.sh teardown
./test/integration/run.sh cluster-down   # optional: destroy the kind cluster
```

### Refreshing Image Content

When you've made changes to the HyperShift codebase and need to re-deploy the operators, run the following -
a new image will be built, loaded into the cluster, and all `Pod`s deploying the image will be deleted, so
the new image is picked up on restart.

```shell
./test/integration/run.sh \
  image \ # build the container image for HyperShift, load it into the cluster
  reload  # power-cycle all pods that should be running the image
```
