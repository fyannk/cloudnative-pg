#!/bin/bash

NAMESPACE="default"
SECRET_PREFIX="huge-secret"
CONFIGMAP_PREFIX="huge-configmap"
NBTOCREATE="100"

F_Generate()
{
  if [ ! -f "$SECRET_PREFIX.yaml" ]; then
    for i in $(seq 1 300); do
      echo "ENVVAR${i}=${i}" >> $SECRET_PREFIX.yaml
    done
  fi
  if [ ! -d ../config/crd/bases ]; then
    echo "CRDs not found"
    exit 1
  fi
}

F_Create()
{
  F_Generate
  for i in $(seq 1 $NBTOCREATE); do
    kubectl create secret generic "${SECRET_PREFIX}-${i}" --from-file=../config/crd/bases -n $NAMESPACE
    kubectl create configmap "${CONFIGMAP_PREFIX}-${i}" --from-env-file="$SECRET_PREFIX.yaml" -n $NAMESPACE
  done
}

F_Delete()
{
  for i in $(seq 1 $NBTOCREATE); do
    kubectl delete secret "${SECRET_PREFIX}-${i}" -n $NAMESPACE
    kubectl delete configmap "${CONFIGMAP_PREFIX}-${i}" -n $NAMESPACE
  done
}

F_Test()
{
  F_Create
  sleep 10m
  F_Delete
}

F_Usage()
{
  echo "Usage: $0 [-n <namespace>] [-b <number of objects to create>] -a {generate|create|delete|test}"
  exit 1
}

if [ ! -f /usr/bin/kubectl ]; then
  echo "kubectl not found"
  exit 1
fi

getopts ":a:n:" opt
case $opt in
  n)
    NAMESPACE=$OPTARG
    ;;
  b)
    NBTOCREATE=$OPTARG
    ;;
  a)
    case "$OPTARG" in
      generate)
        F_Generate
        ;;
      create)
        F_Create
        ;;
      delete)
        F_Delete
        ;;
      test)
        F_Test
        ;;
      *)
        F_Usage
    esac
    ;;
  h)
    F_Usage
    ;;
  *)
    F_Usage
    ;; 
esac
