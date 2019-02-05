FROM golang:1.11-alpine as builder

WORKDIR /go/src/github.com/klihub/cpu-policy-plugins-for-kubernetes
COPY . .
RUN cd cmd/cpu-pool-admissionwebhook; go install
RUN chmod a+x /go/bin/cpu-pool-admissionwebhook

FROM alpine
COPY --from=builder /go/bin/cpu-pool-admissionwebhook /usr/bin
CMD ["/usr/bin/cpu-pool-admissionwebhook"]
