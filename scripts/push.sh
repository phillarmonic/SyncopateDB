#!/usr/bin/env bash
# Run on the repo: git config core.sshCommand "./scripts/push.sh"
exec ssh -i ~/.ssh/id_phillarmonic -F /dev/null "$@"
