#!/usr/bin/env bash
kubectl delete pods -A --all --force --grace-period=0 --wait=false
kubectl delete services -A --all --force --grace-period=0 --wait=false
kubectl delete endpoints -A --all --force --grace-period=0 --wait=false