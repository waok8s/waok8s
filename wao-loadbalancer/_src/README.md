# _src

This repository contains patches files.

```
.
├── cmd
│   └── kube-proxy               # overwrite `k8s.io/kubernetes/cmd/kube-proxy`
│       ├── app
│       │   ├── server.go        # (modified) label selector
│       │   ├── server_linux.go  # (modified) changed import path of `nftables`, edited `platformApplyDefaults`
│       │   └── wao.go           # (added) consts for WAO
│       └── proxy.go             # (modified) changed import path of `app`
├── pkg
│   └── proxy                    # overwrite `k8s.io/kubernetes/pkg/proxy`
│       └── nftables
│           ├── proxier.go       # (modified) use WAO to calculate weight
│           ├── wao.go           # (add) the WAO implementation
│           └── wao_test.go      # (add) tests for WAO
└── README.md
```
