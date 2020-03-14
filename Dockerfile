# Build the manager binary
FROM golang:1.11.5-alpine3.8 as builder

# Copy in the go src
WORKDIR /go/src/github.com/iyacontrol/cronhpa
COPY pkg/    pkg/
COPY main.go  main.go
COPY vendor/ vendor/

# Build
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -a -o manager

# Copy the controller-manager into a thin image
FROM alpine:3.8
RUN apk update \
    && apk add tzdata \
    && cp /usr/share/zoneinfo/Asia/Shanghai /etc/localtime \
    && echo "Asia/Shanghai" > /etc/timezone
WORKDIR /root/
COPY --from=builder /go/src/github.com/iyacontrol/cronhpa/manager .
ENTRYPOINT ["./manager"]