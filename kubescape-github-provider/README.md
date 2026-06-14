# kubescape-github-provider

A minimal Kubescape Synchronizer-compatible provider that receives SBOMSyft / SBOMSyftFiltered objects and commits them into a GitHub repository.

## Environment variables

Required:

- `GITHUB_TOKEN`: GitHub fine-grained PAT or GitHub App installation token with repository Contents read/write permission.
- `GITHUB_OWNER`: repository owner or organization.
- `GITHUB_REPO`: repository name.

Optional:

- `GITHUB_BRANCH`: branch to update. Default: `main`.
- `GITHUB_PATH_PREFIX`: directory prefix. Default: `kubescape-sbom`.
- `PROVIDER_ACCESS_KEY`: if set, incoming Kubescape Synchronizer connections must send the same value as `X-API-KEY`.
- `PORT`: default `8080`.

## Run locally

```bash
export GITHUB_TOKEN=github_pat_xxx
export GITHUB_OWNER=my-org
export GITHUB_REPO=security-artifacts
export GITHUB_BRANCH=main
export PROVIDER_ACCESS_KEY='replace-me'
go run .
```

Expose it as `wss://.../` and configure Kubescape Operator with:

```bash
helm upgrade --install kubescape kubescape/kubescape-operator \
  -n kubescape --create-namespace \
  --set clusterName="$(kubectl config current-context)" \
  --set server="wss://kubescape-github-provider.example.com/" \
  --set account="github-provider" \
  --set accessKey="replace-me" \
  --set capabilities.nodeSbomGeneration=enable \
  --set capabilities.syncSBOM=enable
```

## Notes

- The provider requests the full object whenever it receives a `newChecksum` for `sbomsyfts` or `sbomsyftfiltereds`.
- Files are committed under `<GITHUB_PATH_PREFIX>/<cluster>/<namespace>/<sbom-name>.json`.
- `patchObject` messages are not applied; the provider asks for the full object again.
- For production, terminate TLS at an Ingress or load balancer and restrict network access.


```
GITHUB_TOKEN=<YOUR TOKEN>
```

```
kubectl create ns kubescape-github-provider
kubectl apply -f - <<EOF
apiVersion: v1
kind: Secret
metadata:
  name: kubescape-github-provider
  namespace: kubescape-github-provider
type: Opaque
stringData:
  GITHUB_TOKEN: "${GITHUB_TOKEN}"
  PROVIDER_ACCESS_KEY: provider-access-key
EOF
```
