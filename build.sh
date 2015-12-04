#!/bin/bash
set -e
set -x

VERSION=`git rev-parse --short HEAD`
NAME=`basename ${PWD}`

CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o bin/${NAME}

TAG="${NAME}:${VERSION}"
if [ -n "${DOCKER_REPO}" ]; then
    TAG="${DOCKER_REPO}/${TAG}"
fi
docker build -t ${TAG} .

if [ -n "${DOCKER_REPO}" ]; then
    docker push ${TAG}
fi
