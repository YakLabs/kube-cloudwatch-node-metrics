FROM debian:jessie

RUN apt-get update && \
    apt-get install -y curl bash jq && \
    apt-get clean -y all

RUN cd /tmp && \
    curl -L -O https://storage.googleapis.com/kubernetes-release/release/v1.1.2/bin/linux/amd64/kubectl && \
    mv kubectl /usr/bin/ && \
    chmod 0555 /usr/bin/kubectl

COPY ./start /

RUN chmod 0555 /start

COPY ./bin/kube-cloudwatch-node-metrics /usr/bin/
