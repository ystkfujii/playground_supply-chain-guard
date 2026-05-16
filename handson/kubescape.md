```
curl -s https://raw.githubusercontent.com/kubescape/kubescape/master/install.sh | /bin/bash
export PATH=$PATH:/home/ystkfujii/.kubescape/bin
kubescape version
```

```
kubescape scan --keep-local
```

個別スキャン

```
kubescape scan framework nsa --keep-local
kubescape scan framework mitre --keep-local
kubescape scan framework cis-v1.23-t1.0.1 --keep-local
```


```
kubectl get -n kubescape sbomsyfts \
  -L kubescape.io/image-name,kubescape.io/workload-namespace,kubescape.io/workload-name,kubescape.io/workload-kind
```

```
kubectl get sbomsyfts -n kubescape ghcr.io-ystkfujii-playground-supply-chain-guard-go-server-sha256-7a3aedeead478347a51f5a3ba0182f4b4700b709c3b128cbec5bbf2a65724453-724453 -o yaml > out.yaml
```

go-moddule typeで確認できる。

たぶんgo versionの情報

```
 go version -m go-server 
go-server: go1.26.2
        path    github.com/ystkfujii/playground_supply-chain-guard/go-server
        mod     github.com/ystkfujii/playground_supply-chain-guard/go-server    (devel)
        dep     github.com/labstack/echo/v4     v4.15.2 h1:nnh2sCzGCVYnU+wCisMPiYapEg/QVo/gcI9ePKg5/T4=
        dep     github.com/labstack/gommon      v0.5.0  h1:6VSQ2NOzsnEJ5W6+84E0RbcaDDmgB6NIAzWCczTEe6c=
        dep     github.com/mattn/go-colorable   v0.1.14 h1:9A9LHSqF/7dyVVX6g0U9cwm9pG3kP9gSzcuIPHPsaIE=
        dep     github.com/mattn/go-isatty      v0.0.22 h1:j8l17JJ9i6VGPUFUYoTUKPSgKe/83EYU2zBC7YNKMw4=
        dep     github.com/valyala/bytebufferpool       v1.0.0  h1:GqA5TC/0021Y/b9FG4Oi9Mr3q7XYx6KllzawFIhcdPw=
        dep     github.com/valyala/fasttemplate v1.2.2  h1:lxLXG0uE3Qnshl9QyaK6XJxMXlQZELvChBOCmQD0Loo=
        dep     golang.org/x/crypto     v0.50.0 h1:zO47/JPrL6vsNkINmLoo/PH1gcxpls50DNogFvB5ZGI=
        dep     golang.org/x/net        v0.53.0 h1:d+qAbo5L0orcWAr0a9JweQpjXF19LMXJE8Ey7hwOdUA=
        dep     golang.org/x/sys        v0.43.0 h1:Rlag2XtaFTxp19wS8MXlJwTvoh8ArU6ezoyFsMyCTNI=
        dep     golang.org/x/text       v0.36.0 h1:JfKh3XmcRPqZPKevfXVpI1wXPTqbkE5f7JA92a55Yxg=
        dep     golang.org/x/time       v0.15.0 h1:bbrp8t3bGUeFOx08pvsMYRTCVSMk89u4tKbNOZbp88U=
        build   -buildmode=exe
        build   -compiler=gc
        build   CGO_ENABLED=1
        build   CGO_CFLAGS=
        build   CGO_CPPFLAGS=
        build   CGO_CXXFLAGS=
        build   CGO_LDFLAGS=
        build   GOARCH=amd64
        build   GOOS=linux
        build   GOAMD64=v1
        build   vcs=git
        build   vcs.revision=bc28a16a0d51d63469990fe0701c50e970c26efa
        build   vcs.time=2026-05-17T10:59:32Z
        build   vcs.modified=true
```


imageスキャン

```
kubescape scan image nginx:1.21 -v
```


Manifestスキャン

```
kubescape scan ./manifests --keep-local
```

CRD

```
$ kubectl api-resources | grep kubesca
cloudproviderinfos                                       hostdata.kubescape.cloud/v1beta1                false        CloudProviderInfo
cniinfos                                                 hostdata.kubescape.cloud/v1beta1                false        CNIInfo
controlplaneinfos                                        hostdata.kubescape.cloud/v1beta1                false        ControlPlaneInfo
kernelversions                                           hostdata.kubescape.cloud/v1beta1                false        KernelVersion
kubeletinfos                                             hostdata.kubescape.cloud/v1beta1                false        KubeletInfo
kubeproxyinfos                                           hostdata.kubescape.cloud/v1beta1                false        KubeProxyInfo
linuxkernelvariables                                     hostdata.kubescape.cloud/v1beta1                false        LinuxKernelVariables
linuxsecurityhardeningstatuses                           hostdata.kubescape.cloud/v1beta1                false        LinuxSecurityHardeningStatus
openportslists                                           hostdata.kubescape.cloud/v1beta1                false        OpenPortsList
osreleasefiles                                           hostdata.kubescape.cloud/v1beta1                false        OsReleaseFile
clustersecurityexceptions             cse                kubescape.io/v1beta1                            false        ClusterSecurityException
operatorcommands                      opcmd              kubescape.io/v1alpha1                           true         OperatorCommand
rules                                                    kubescape.io/v1                                 true         Rules
runtimerulealertbindings              rab                kubescape.io/v1                                 false        RuntimeRuleAlertBinding
seccompprofiles                       scp                kubescape.io/v1                                 true         SeccompProfile
securityexceptions                    se                 kubescape.io/v1beta1                            true         SecurityException
servicesscanresults                   kssa               kubescape.io/v1                                 true         ServiceScanResult
applicationprofiles                                      spdx.softwarecomposition.kubescape.io/v1beta1   true         ApplicationProfile
configurationscansummaries                               spdx.softwarecomposition.kubescape.io/v1beta1   false        ConfigurationScanSummary
containerprofiles                                        spdx.softwarecomposition.kubescape.io/v1beta1   true         ContainerProfile
generatednetworkpolicies                                 spdx.softwarecomposition.kubescape.io/v1beta1   true         GeneratedNetworkPolicy
knownservers                                             spdx.softwarecomposition.kubescape.io/v1beta1   false        KnownServer
networkneighborhoods                                     spdx.softwarecomposition.kubescape.io/v1beta1   true         NetworkNeighborhood
openvulnerabilityexchangecontainers                      spdx.softwarecomposition.kubescape.io/v1beta1   true         OpenVulnerabilityExchangeContainer
sbomsyftfiltereds                                        spdx.softwarecomposition.kubescape.io/v1beta1   true         SBOMSyftFiltered
sbomsyfts                                                spdx.softwarecomposition.kubescape.io/v1beta1   true         SBOMSyft
vulnerabilitymanifests                                   spdx.softwarecomposition.kubescape.io/v1beta1   true         VulnerabilityManifest
vulnerabilitymanifestsummaries                           spdx.softwarecomposition.kubescape.io/v1beta1   true         VulnerabilityManifestSummary
vulnerabilitysummaries                                   spdx.softwarecomposition.kubescape.io/v1beta1   false        VulnerabilitySummary
workloadconfigurationscans                               spdx.softwarecomposition.kubescape.io/v1beta1   true         WorkloadConfigurationScan
workloadconfigurationscansummaries                       spdx.softwarecomposition.kubescape.io/v1beta1   true         WorkloadConfigurationScanSummary
```
