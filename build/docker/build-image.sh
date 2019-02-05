#!/bin/sh -e

IMAGE=$1
DOCKERFILE="$(dirname $0)/$IMAGE.Dockerfile"

usage () {
    usg="$0 image-name"

    if [ -n "$*" ]; then
        echo "$*" 1>&2
        echo "$usg" 1>&2

        exit 1
    else
        echo "$usg" 1>&2

        exit 0
    fi
}

fatal () {
    if [ -n "$*" ]; then
        msg="fatal error: $*"
    else
        msg="fatal error"
    fi

    echo "$msg" 1>&2
    exit 1
}


if [ -z "$IMAGE" ]; then
    usage "mandatory image name not given"
fi

if [ ! -e "$DOCKERFILE" ]; then
    fatal "Dockerfile $DOCKERFILE not found"
fi

TAG=$(git rev-parse HEAD)
docker build -t intel-${IMAGE}:${TAG} -f ${DOCKERFILE} .
docker tag intel-${IMAGE}:${TAG} intel-${IMAGE}:devel
