# syntax=docker/dockerfile:1
# One image, two processes: Impostor (the Among Us server, built here from OpenPak's unmodified
# fork at IMPOSTOR_REF) and the OpenPak front that logs and proxies the matchmaker calls to it.
# They share a container and nothing else: the front talks to Impostor over loopback, which is
# mere aggregation, so the GPL work stays its own program.
ARG IMPOSTOR_REF=openpak-v1.10.8

FROM --platform=$BUILDPLATFORM golang:1.26-alpine AS front
ARG TARGETOS TARGETARCH
WORKDIR /src
COPY go.mod go.su[m] ./
COPY . .
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -ldflags="-s -w" -o /out/among-us ./cmd/among-us

FROM --platform=$BUILDPLATFORM mcr.microsoft.com/dotnet/sdk:8.0 AS impostor
ARG TARGETARCH IMPOSTOR_REF
RUN apt-get update -qq && apt-get install -y -qq git >/dev/null
WORKDIR /source
# openpak/impostor is private: the build passes a netrc secret (never stored in a layer).
RUN --mount=type=secret,id=netrc,target=/root/.netrc \
    git clone -q --depth 1 -b "$IMPOSTOR_REF" https://github.com/openpak/impostor.git .
RUN case "$TARGETARCH" in amd64) P=linux-x64;; arm64) P=linux-arm64;; *) echo "unsupported $TARGETARCH"; exit 1;; esac \
 && dotnet publish -c release -o /app -r "$P" -p:VersionSuffix=openpak ./src/Impostor.Server/Impostor.Server.csproj

FROM --platform=$TARGETPLATFORM mcr.microsoft.com/dotnet/aspnet:8.0
WORKDIR /app
COPY --from=impostor /app ./
COPY --from=front /out/among-us /usr/local/bin/among-us
COPY entrypoint.sh /usr/local/bin/entrypoint.sh
RUN chmod +x /usr/local/bin/entrypoint.sh
ENV ASPNETCORE_URLS= \
    IMPOSTOR_Server__ListenIp=0.0.0.0 IMPOSTOR_Server__ListenPort=22603 IMPOSTOR_Server__PublicPort=22603 \
    IMPOSTOR_HttpServer__ListenIp=127.0.0.1 IMPOSTOR_HttpServer__ListenPort=22601 \
    AMONG_US_HTTP_ADDR=:22600 AMONG_US_IMPOSTOR_URL=http://127.0.0.1:22601
# 22600/tcp: the front (behind Traefik). 22603/udp: game traffic (publish it). Set IMPOSTOR_Server__PublicIp.
EXPOSE 22600/tcp 22603/udp
ENTRYPOINT ["/usr/local/bin/entrypoint.sh"]
