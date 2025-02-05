# _src

This repository contains patches files.

```
.
├── cmd
│   └── kube-proxy               # overwrite `k8s.io/kubernetes/cmd/kube-proxy`
│       ├── app
│       │   └── server_others.go # (modified) changed import path of `nftables`
│       └── proxy.go             # (modified) changed import path of `app`
├── pkg
│   └── proxy                    # overwrite `k8s.io/kubernetes/pkg/proxy`
│       ├── ipvs
│       │   ├── proxier.go       # (modified) use WAO to calculate weight
│       │   └── wao.go           # (add) the WAO implementation
│       └── nftables
│           ├── proxier.go       # (modified) use WAO to calculate weight
│           └── wao.go           # (add) the WAO implementation
└── README.md
```
