```console
$ kubectl api-resources | grep aqua
clustercompliancereports              compliance                         aquasecurity.github.io/v1alpha1                 false        ClusterComplianceReport
clusterconfigauditreports             clusterconfigaudit                 aquasecurity.github.io/v1alpha1                 false        ClusterConfigAuditReport
clusterinfraassessmentreports         clusterinfraassessment             aquasecurity.github.io/v1alpha1                 false        ClusterInfraAssessmentReport
clusterrbacassessmentreports          clusterrbacassessmentreport        aquasecurity.github.io/v1alpha1                 false        ClusterRbacAssessmentReport
clustersbomreports                    clustersbom                        aquasecurity.github.io/v1alpha1                 false        ClusterSbomReport
clustervulnerabilityreports           clustervuln                        aquasecurity.github.io/v1alpha1                 false        ClusterVulnerabilityReport
configauditreports                    configaudit,configaudits           aquasecurity.github.io/v1alpha1                 true         ConfigAuditReport
exposedsecretreports                  exposedsecret,exposedsecrets       aquasecurity.github.io/v1alpha1                 true         ExposedSecretReport
infraassessmentreports                infraassessment,infraassessments   aquasecurity.github.io/v1alpha1                 true         InfraAssessmentReport
rbacassessmentreports                 rbacassessment,rbacassessments     aquasecurity.github.io/v1alpha1                 true         RbacAssessmentReport
sbomreports                           sbom,sboms                         aquasecurity.github.io/v1alpha1                 true         SbomReport
vulnerabilityreports                  vuln,vulns                         aquasecurity.github.io/v1alpha1                 true         VulnerabilityReport
```

```
kubectl get vuln -A -o wide
kubectl get configaudit -A -o wide
kubectl get exposedsecret -A -o wide
kubectl get rbacassessmentreports -A
kubectl get sbomreports -A
```

```
kubectl get -A vuln -l trivy-operator.resource.namespace=demo-node -o yaml
```

```
kubectl get -A vuln -l trivy-operator.resource.namespace=demo-python -o yaml
```


trivy自体はCycloneDXとSPDXに対応しているが、OperatorのFromatはCycloneDXのみっぽい。


https://github.com/aquasecurity/trivy-operator/blob/028dec89e44c60eb98a63fec064b353441436a2e/pkg/sbomreport/io.go#L204
