# Build a static eonyx binary and ship it in a minimal runtime image.
# Pass the version at build time so `eonyx --version` is accurate even
# without a .git directory in the build context:
#   docker build --build-arg VERSION=$(git describe --tags --always --dirty) -t eonyx .

FROM golang:1.26-alpine AS build
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
ARG VERSION=dev
RUN CGO_ENABLED=0 go build \
    -ldflags "-s -w -X eonyx/internal/version.Version=${VERSION}" \
    -o /out/eonyx .

FROM alpine:3.20
# ca-certificates for HTTPS to model APIs; git and bash for the agent's use of
# the in-project working tree.
RUN apk add --no-cache ca-certificates git bash
COPY --from=build /out/eonyx /usr/local/bin/eonyx
WORKDIR /work
ENTRYPOINT ["eonyx"]
