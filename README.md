```
make setup
```


```bash
make build
make push
```

```bash
make digest
```


```bash
IMAGE_DIGEST=$(make digest)
notation list "$IMAGE_DIGEST"
```

```bash
make cert
```


```bash
make policy
```

```
gh attestation verify "oci://${IMAGE}"   -R ystkfujii/playground_supply-chain-guard   --signer-workflow ystkfujii/playground_supply-chain-guard/.github/workflows/build_image.yml   --source-ref refs/heads/main
```

```
gh attestation trusted-root \
| jq '. | select(.certificateAuthorities[0].uri == "fulcio.githubapp.com")' \
> github-trusted-root.json
```

```
cosign verify-blob-attestation \
  --bundle bundle.json \
  --new-bundle-format \
  --trusted-root github-trusted-root.json \
  --use-signed-timestamps \
  --insecure-ignore-tlog=true \
  --insecure-ignore-sct \
  --type=slsaprovenance1 \
  --certificate-oidc-issuer="https://token.actions.githubusercontent.com" \
  --certificate-identity-regexp='^https://github\.com/ystkfujii/playground_supply-chain-guard/\.github/workflows/build_image\.yml@refs/heads/main$' \
  --digest "${DIGEST#sha256:}" \
  --digestAlg sha256
```

```
export ARGOCD_VERSION=v3.4.2
kubectl create namespace argocd

kubectl apply -n argocd --server-side --force-conflicts \
  -f "https://raw.githubusercontent.com/argoproj/argo-cd/${ARGOCD_VERSION}/manifests/install.yaml"

kubectl -n argocd rollout status deploy/argocd-server --timeout=300s
kubectl -n argocd rollout status deploy/argocd-repo-server --timeout=300s
kubectl -n argocd rollout status statefulset/argocd-application-controller --timeout=300s
```
