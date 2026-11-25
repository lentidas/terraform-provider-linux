#!/bin/sh
# Prepares the credentials the acceptance tests log in with, then hands over to sshd (or whatever CMD says).
#
#   SSH_ROOT_PASSWORD         password of root (default: root, the same default the tests use)
#   /run/test-authorized-keys optional public key(s) to authorize for root, mounted read-only

set -eu

# Generate new host keys on every start (the tests do not pin them).
ssh-keygen -A > /dev/null

# Set the root password for the acceptance tests (default: root)
echo "root:${SSH_ROOT_PASSWORD:-root}" | chpasswd

# Copy the authorized keys file instead of using it in place, because sshd refuses key files with the ownership of a bind mount.
if [ -f /run/test-authorized-keys ]; then
  install -D -m 600 -o root -g root /run/test-authorized-keys /root/.ssh/authorized_keys
fi

exec "$@"
