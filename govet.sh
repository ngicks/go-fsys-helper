#!/bin/bash

supported_list=$(go tool dist list)

IFS=$'\n'
for os_arch in $supported_list; do
  IFS='/' read -r os arch <<< $os_arch
  if [[ $os == "android" ]] || [[ $os == "ios" ]]; then
    continue
  fi
  echo ${os_arch}:
  GOOS=${os} GOARCH=${arch} go vet ./...
  echo
done
