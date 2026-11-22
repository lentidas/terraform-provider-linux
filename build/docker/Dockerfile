# syntax=docker/dockerfile:1

# Disposable SSH server used as the target of the acceptance tests. It is never meant to hold anything of value: root
# can log in with a known password and with the key in linux/testdata, so only ever publish its port on loopback.
ARG DEBIAN_IMAGE=debian:trixie-slim@sha256:a72b4433de1cf7d6bae0c4cee74d0547f58601a643b3375f070c51600d63b6b2

FROM ${DEBIAN_IMAGE}

ARG DEBIAN_IMAGE

LABEL org.opencontainers.image.title="terraform-provider-linux-test-sshd" \
      org.opencontainers.image.description="Disposable SSH server for the terraform-provider-linux acceptance tests" \
      org.opencontainers.image.vendor="lentidas" \
      org.opencontainers.image.licenses="MPL-2.0" \
      org.opencontainers.image.url="https://github.com/lentidas/terraform-provider-linux" \
      org.opencontainers.image.source="https://github.com/lentidas/terraform-provider-linux" \
      org.opencontainers.image.documentation="https://github.com/lentidas/terraform-provider-linux#developing-the-provider" \
      org.opencontainers.image.base.name="${DEBIAN_IMAGE}"

# openssh-client provides the scp binary the provider's communicator calls on the server to upload files. The tests
# validate their results with `[ a == b ]` in scripts that Terraform runs through /bin/sh, which dash (the Debian default)
# rejects, so this image points /bin/sh at bash.
RUN set -eux; \
    apt-get update && \
    apt-get install -y --no-install-recommends \
        openssh-client \
        openssh-server && \
    rm -rf /var/lib/apt/lists/* && \
    mkdir -p /run/sshd && \
    ln -sf bash /bin/sh

COPY sshd_config.conf /etc/ssh/sshd_config.d/10-acceptance-tests.conf
COPY --chmod=755 entrypoint.sh /usr/local/bin/entrypoint.sh

EXPOSE 22

# One connection per probe: sshd 10 penalizes sources that connect without authenticating, and ssh-keyscan without -t
# opens one connection per key type.
HEALTHCHECK --interval=3s --timeout=3s --start-period=2s --retries=15 \
    CMD ssh-keyscan -T 2 -t ed25519 127.0.0.1 2>/dev/null | grep -q .

# Deliberately runs as root: sshd needs it to switch users, and the tests log in as root to exercise ownership changes.
ENTRYPOINT ["/usr/local/bin/entrypoint.sh"]
CMD ["/usr/sbin/sshd", "-D", "-e"]
