#!/bin/bash

function finished {
    set +x
    echo
    echo "Finished $(date +%F-%H%M%S)"
}
trap finished EXIT

function header {
    local msg="$1"
    echo
    echo "$msg" | sed 's/./=/g'
    echo $msg
    echo "$msg" | sed 's/./=/g'
    echo
}
