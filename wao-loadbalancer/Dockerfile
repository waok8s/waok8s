FROM golang:1.23
WORKDIR /go/src/app

# cache go modules
COPY go.mod .
COPY go.sum .
RUN go mod download

# copy files and build
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 make build

FROM registry.k8s.io/kube-proxy:v1.30.0
COPY --from=0 /go/src/app/bin/kube-proxy /usr/local/bin/kube-proxy
CMD ["kube-proxy"]
