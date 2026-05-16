
```
helm repo add guacsec https://guacsec.github.io/helm-charts
helm repo update
helm search repo guacsec
```


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
