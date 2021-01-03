FROM golang:1.15 as builder

ENV GOPATH=/opt
RUN go get github.com/nazavode/rm/cmd/rmd 

FROM ubuntu:20.04

COPY --from=builder /opt/bin/rmd /opt/bin/rmd
RUN apt-get -yqq update && apt-get -yqq upgrade \
 && apt-get -yqq install pandoc ca-certificates \
 && rm -rf /var/lib/apt/lists/* \
 && update-ca-certificates
ENTRYPOINT ["/opt/bin/rmd"]
