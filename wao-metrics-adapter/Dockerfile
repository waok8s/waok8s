FROM golang:1.23
WORKDIR /go/src/app

# cache go modules
COPY go.mod .
COPY go.sum .
RUN go mod download

# copy files and build
COPY . .
# this enables `docker build --build-arg VERSION=...` for injecting version without `.git` directory in build context; See Makefile for more details
ARG VERSION
RUN VERSION=$VERSION CGO_ENABLED=0 GOOS=linux GOARCH=amd64 make build

FROM gcr.io/distroless/static-debian11
COPY --from=0 /go/src/app/bin /bin
