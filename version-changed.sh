#!/usr/bin/env bash

BRANCH_NAME=${GITHUB_REF##*/}

if [ "${BRANCH_NAME}" = "master" ]; then
  echo "no need check master"
else
  echo "check if version was changed"
  git cat-file -e origin/master:version
  ret=$?
  if [[ ${ret} -eq 0 ]]; then
    git diff origin/master version | grep +baseVersion
  else
    echo "Error: 'version' file does not exist in origin/master."
    exis $0
#    exit ${ret}
  fi
fi
