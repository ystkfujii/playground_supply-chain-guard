# sbomreport-github-sync

Trivy Operator が生成する `SbomReport` リソースを Kubernetes API から取得し、GitHub リポジトリに JSON として同期する CLI です。

- Kubernetes には in-cluster config、または `KUBECONFIG` で接続します。
- GitHub には `GITHUB_TOKEN` などの環境変数で接続します。
- GitHub への反映は Git Database API を使い、1 回の同期を 1 commit にまとめます。
- デフォルトでは `path-prefix` 配下の古い `.json` を削除して、現在の `SbomReport` 一覧と同期します。

## Build

```bash
go mod tidy
go build -o sbomreport-github-sync .
```

## Local run

```bash
export KUBECONFIG=$HOME/.kube/config
export GITHUB_TOKEN=github_pat_xxx
export GITHUB_OWNER=your-org
export GITHUB_REPO=sbom-archive
export GITHUB_BRANCH=main
export GITHUB_PATH_PREFIX=clusters/dev/sbomreports
export CLUSTER_NAME=dev

./sbomreport-github-sync sync --dry-run
./sbomreport-github-sync sync
```

## Options

```bash
sbomreport-github-sync sync \
  --namespace default \
  --selector 'trivy-operator.resource.kind=Deployment' \
  --content cyclonedx \
  --path-prefix clusters/prod/sbomreports
```

`--content` は次のいずれかです。

- `cyclonedx`: `.report.components` だけを書き出します。デフォルトです。
- `report`: `.report` 全体を書き出します。
- `resource`: `SbomReport` Custom Resource 全体を書き出します。

## Required GitHub permissions

Fine-grained PAT の場合は、対象リポジトリに対する `Contents: Read and write` 権限を付けてください。
Git Database API で blob/tree/commit/ref を操作します。

## Environment variables

| Name | Required | Default | Description |
| --- | --- | --- | --- |
| `GITHUB_TOKEN` | yes | | GitHub token |
| `GITHUB_OWNER` | yes | | GitHub repository owner |
| `GITHUB_REPO` | yes | | GitHub repository name |
| `GITHUB_BRANCH` | no | `main` | Branch to update |
| `GITHUB_PATH_PREFIX` | no | `sbomreports` | Directory prefix in repository |
| `GITHUB_API_URL` | no | `https://api.github.com` | API URL for GitHub or GitHub Enterprise |
| `GITHUB_API_VERSION` | no | `2026-03-10` | REST API version header |
| `KUBECONFIG` | no | | Local kubeconfig path |
| `NAMESPACE` | no | all namespaces | Namespace to list |
| `LABEL_SELECTOR` | no | | Label selector |
| `CLUSTER_NAME` | no | | Cluster name for index metadata |
| `SBOMREPORT_CONTENT` | no | `cyclonedx` | `cyclonedx`, `report`, or `resource` |
| `DELETE_MISSING` | no | `true` | Delete stale `.json` under path prefix |
| `INCLUDE_INDEX` | no | `true` | Write `index.json` |
| `FAIL_IF_EMPTY` | no | `false` | Fail when no reports are found |

## Kubernetes manifest

See `deploy/cronjob.yaml`.

Apply after replacing placeholders:


```
GITHUB_TOKEN=<YOUR TOKEN>
```

```
kubectl create ns trivy-system
kubectl apply -f - <<EOF
apiVersion: v1
kind: Secret
metadata:
  name: sbomreport-github-sync
  namespace: trivy-system
type: Opaque
stringData:
  github-token: "${GITHUB_TOKEN}"
EOF
```


```bash
kubectl apply -f deploy/cronjob.yaml
```

```
kubectl create job \
  --from=cronjob/sbomreport-github-sync \
  -n trivy-system \
  sbomreport-github-sync-manual-$(date +%s)
```

