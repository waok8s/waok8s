FROM golang:1.19
WORKDIR /go/src/app
# cache go modules
COPY go.mod .
COPY go.sum .
RUN go mod download
# copy files and build
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 make build-bin

FROM gcr.io/distroless/static-debian11
COPY --from=0 /go/src/app/bin/kube-scheduler /usr/local/bin/kube-scheduler
CMD ["kube-scheduler"]
