#!/usr/bin/env bash

set -e

echo_info() {
  echo -e "\033[34m[INFO] $1\033[0m"
}

echo_note() {
  echo -e "\033[33m$1\033[0m"
}

if [ -z $CLUSTER_NAME ]; then
    echo_note "CLUSTER_NAME environment variable was not provided"
    exit 1
fi

if [ -z $CLUSTER_REGION ]; then
	echo_note "CLUSTER_REGION environment variable was not provided"
	exit 1
fi

export AWS_REGION=${CLUSTER_REGION}

echo_info "This script is used to restore the cluster $CLUSTER_NAME's auto-scaling-groups in region $AWS_REGION"

NODEGROUPS=$(aws autoscaling describe-auto-scaling-groups --query \
    "AutoScalingGroups[].AutoScalingGroupName" --filters "Name=tag:eks:cluster-name,Values=${CLUSTER_NAME}" --output text)

if [ -z "$NODEGROUPS" ]; then
    echo_note "No autoscaling groups found"
    exit 0
fi

TOTAL_NODES=0
for NODEGROUP in $NODEGROUPS; do
    # get current desired size
    DESIRED_SIZE=$(aws autoscaling describe-auto-scaling-groups --auto-scaling-group-name $NODEGROUP --query "AutoScalingGroups[].DesiredCapacity" --output text)
    # check the DESIRED_SIZE is non-negative
    if ! [[ "$NEW_DESIRED_SIZE" =~ ^[0-9]+$ ]]; then
        echo_note "Invalid input. Desired size should be a non-negative integer."
        exit 1
    fi
    echo_info "Start to update autoscaling group $NODEGROUP's desired size to $NEW_DESIRED_SIZE"
    TOTAL_NODES=$((TOTAL_NODES + NEW_DESIRED_SIZE))
    aws autoscaling update-auto-scaling-group --auto-scaling-group-name $NODEGROUP --desired-capacity $NEW_DESIRED_SIZE
done

echo_note "All autoscaling groups have been updated"

MAX_RETRIES=30
RETRY_INTERVAL=10

for ((i = 1; i <= MAX_RETRIES; i++)); do
    echo_info "Attempt $i/$MAX_RETRIES: Checking if autoscaling groups ready nodes match desired size $TOTAL_NODES"

    READY_NODES=$(kubectl get node -l node.cloudpilot.ai/managed!=true 2>/dev/null | grep -v 'NotReady' | grep Ready | wc -l | tr -d ' ')

    if [ "$READY_NODES" -eq "$TOTAL_NODES" ]; then
        echo_info "Success: autoscaling groups ready node count matches desired size $READY_NODES"
        break
    fi

    if [ "$i" -eq "$MAX_RETRIES" ]; then
        echo_note "Error: Timeout waiting for autoscaling groups ready node count to reach $TOTAL_NODES. Current count: $READY_NODES"
        exit 1
    fi

    echo_info "Current ready autoscaling groups node count: $READY_NODES. Retrying in $RETRY_INTERVAL seconds..."
    sleep "$RETRY_INTERVAL"
done

echo_note "All autoscaling groups node are ready"
